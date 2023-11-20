package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	message "github.com/emjakobsen1/dsysmutex/proto"
	"google.golang.org/grpc"
)

type msg struct {
	id      int32
	lamport int32
}

type peer struct {
	message.UnimplementedServiceServer
	id      int32
	mutex   sync.Mutex
	replies map[int32]bool
	held    chan bool
	state   string
	lamport int32
	// a "queue"
	queue []msg
	// Update on this channel, when messages should be dequeued
	reply   chan bool
	clients map[int32]message.ServiceClient
	ctx     context.Context
}

func main() {
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + 5000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &peer{
		id:      ownPort,
		clients: make(map[int32]message.ServiceClient),
		replies: make(map[int32]bool),
		held:    make(chan bool),
		ctx:     ctx,
		state:   "RELEASED",
		lamport: 0,
		reply:   make(chan bool),
	}

	for peer := range p.replies {
		p.replies[peer] = false
	}

	// Create listener tcp on port ownPort
	list, err := net.Listen("tcp", fmt.Sprintf(":%v", ownPort))
	if err != nil {
		log.Fatalf("Could not listen on port: %v", err)
	}

	f, cs := setLog(ownPort)
	defer f.Close()

	grpcServer := grpc.NewServer()
	message.RegisterServiceServer(grpcServer, p)

	go func() {
		if err := grpcServer.Serve(list); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	for i := 0; i < 3; i++ {
		port := 5000 + int32(i)

		if port == ownPort {
			continue
		}

		var conn *grpc.ClientConn
		log.Printf("Attempting dial: %v\n", port)

		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Fatalf("Dial failed: %s", err)
		}
		defer conn.Close()
		c := message.NewServiceClient(conn)
		p.clients[port] = c
	}

	log.Printf("Connected to all clients\n")
	//Small delay needed for the clients to establish connection before simulating mutual excl can begin.
	time.Sleep(5 * time.Second)

	go func() {
		for {
			// wait for update
			<-p.reply
			p.mutex.Lock()
			for _, msg := range p.queue {
				p.lamport = max(msg.lamport, p.lamport)
				p.replies[msg.id] = false
				log.Printf("(%v, %v) Send | Allowing %v to enter critical section (queue release) \n", p.id, p.lamport, msg.id)
				p.clients[msg.id].Reply(p.ctx, &message.Info{Id: p.id})
			}
			p.lamport++
			p.queue = nil
			p.state = "RELEASED"
			p.mutex.Unlock()
		}
	}()

	rand.Seed(time.Now().UnixNano() / int64(ownPort))

	// This loop can be uncommented to illustrate 1/100 chance to want access. Instead of every peer spamming "wanted"
	/*for {

		if rand.Intn(100) == 1 {
			p.mutex.Lock()
			p.state = "WANTED"
			p.mutex.Unlock()
			p.enter()
			// wait for state to be held
			<-p.held
			p.lamport++
			log.Printf("(%v, %v) Internal | Enters critical section\n", p.id, p.lamport)
			writeToFile(cs, p.id, p.lamport, "enters critical section.")
			var random = rand.Intn(5)
			time.Sleep(time.Duration(random) * time.Second)
			p.lamport++
			log.Printf("(%v, %v) Internal | Leaves critical section\n", p.id, p.lamport)
			writeToFile(cs, p.id, p.lamport, "leaves critical section.")
			p.reply <- true
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
	}*/

	for {
		p.mutex.Lock()
		p.state = "WANTED"
		p.mutex.Unlock()
		p.enter()
		// wait for state to be held
		<-p.held
		p.lamport++
		log.Printf("(%v, %v) Internal | Entered critical section\n", p.id, p.lamport)
		writeToFile(cs, p.id, p.lamport, "enters critical section.")
		var random = rand.Intn(5)
		time.Sleep(time.Duration(random) * time.Second)
		p.lamport++
		log.Printf("(%v, %v) Internal | Leaving critical section\n", p.id, p.lamport)
		writeToFile(cs, p.id, p.lamport, "leaves critical section.")
		p.reply <- true
		time.Sleep(100 * time.Millisecond)
	}

}

func (p *peer) Request(ctx context.Context, req *message.Info) (*message.Empty, error) {
	p.mutex.Lock()

	if p.state == "HELD" || (p.state == "WANTED" && ((p.lamport < req.Lamport) || (p.lamport == req.Lamport && p.id < req.Id))) {
		p.lamport = max(req.Lamport, p.lamport) + 1
		log.Printf("(%v, %v) Recv | queueing request from %v\n", p.id, p.lamport, req.Id)

		p.queue = append(p.queue, msg{id: req.Id, lamport: req.Lamport})
	} else {

		go func() {

			p.mutex.Lock()
			p.lamport = max(req.Lamport, p.lamport) + 1
			time.Sleep(10 * time.Millisecond)
			//p.lamport++
			p.replies[req.Id] = false
			log.Printf("(%v, %v) Send | Allowing %v to enter critical section\n", p.id, p.lamport, req.Id)
			p.clients[req.Id].Reply(p.ctx, &message.Info{Id: p.id, State: p.state})
			p.mutex.Unlock()

		}()
	}

	p.mutex.Unlock()
	return &message.Empty{}, nil
}

func (p *peer) Reply(ctx context.Context, req *message.Info) (*message.Empty, error) {
	p.mutex.Lock()
	p.lamport = max(p.lamport, req.Lamport) + 1
	log.Printf("(%v, %v) Recv | Got reply from id %v\n", p.id, p.lamport, req.Id)
	p.replies[req.Id] = true

	// Added this check in case a the peer responded but that peer itself is in wanted state.
	// For this case, the responding peer that won access enqueues the request and responds when it is done,
	// since requests are only issued once the peer that lost will eventually get it when it is released from the queue.

	// if req.State == "WANTED" {
	// 	p.queue = append(p.queue, msg{id: req.Id, lamport: req.Lamport})
	// }

	if allTrue(p.replies) {
		p.state = "HELD"
		for peer := range p.replies {
			p.replies[peer] = false
		}
		p.mutex.Unlock()
		p.held <- true
	} else {
		p.mutex.Unlock()
	}
	return &message.Empty{}, nil
}

func allTrue(m map[int32]bool) bool {
	for _, value := range m {
		if !value {
			return false
		}
	}
	return true
}

func (p *peer) enter() {
	p.lamport++
	log.Printf("(%v, %v) Send | Seeking critical section access", p.id, p.lamport)
	info := &message.Info{Id: p.id, Lamport: p.lamport, State: p.state}
	for id, client := range p.clients {

		_, err := client.Request(p.ctx, info)
		if err != nil {
			log.Printf("something went wrong with id %v\n", id)
		}
	}
}

func max(a int32, b int32) int32 {
	if a > b {
		return a
	} else {
		return b
	}
}

func writeToFile(logFile *os.File, id int32, lamport int32, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, err := fmt.Fprintf(logFile, "[%s] (Lamport: %d) %v %s\n", timestamp, lamport, id, message)
	if err != nil {
		log.Printf("Failed to write to critical section log: %v", err)
	}
}

func setLog(port int32) (*os.File, *os.File) {
	// Clears the log.txt file when a new server is started
	filename := fmt.Sprintf("logs/peer(%v)-log.txt", port)
	if err := os.Truncate(filename, 0); err != nil {
		log.Printf("Failed to truncate: %v\n", err)
	}

	// This connects to the log file/changes the output of the log informaiton to the log.txt file.
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	// print to both file and console
	mw := io.MultiWriter(os.Stdout, f)
	log.SetFlags(0)
	log.SetOutput(mw)

	cslog, err := os.OpenFile("logs/cs-log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("Error opening critical section log file: %v", err)
	}

	return f, cslog
}
