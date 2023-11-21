package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
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
	id       int32
	mutex    sync.Mutex
	replies  map[int32]bool
	held     chan bool
	state    string
	lamport  int32
	last_Req int32
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
		id:       ownPort,
		clients:  make(map[int32]message.ServiceClient),
		replies:  make(map[int32]bool),
		held:     make(chan bool),
		ctx:      ctx,
		state:    "RELEASED",
		lamport:  0,
		last_Req: 0,
		reply:    make(chan bool),
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
			accessRessource(cs, p.id, p.lamport, "enters critical section.")
			var random = rand.Intn(5)
			time.Sleep(time.Duration(random) * time.Second)
			p.lamport++
			log.Printf("(%v, %v) Internal | Leaves critical section\n", p.id, p.lamport)
			accessRessource(cs, p.id, p.lamport, "leaves critical section.")
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
		// wait for state to be held, write to file
		<-p.held
		p.lamport++

		log.Printf("(%v, %v) Internal | Entered critical section\n", p.id, p.lamport)
		accessRessource(cs, p.id, p.lamport, "enters critical section.")
		var random = rand.Intn(5)
		time.Sleep(time.Duration(random) * time.Second)
		p.lamport++
		log.Printf("(%v, %v) Internal | Leaving critical section\n", p.id, p.lamport)
		accessRessource(cs, p.id, p.lamport, "leaves critical section.")
		p.reply <- true //exit()
		time.Sleep(100 * time.Millisecond)
	}

}

func (p *peer) Request(ctx context.Context, req *message.Info) (*message.Empty, error) {
	p.mutex.Lock()
	p.lamport = max(req.Lamport, p.lamport)
	if p.state == "HELD" || (p.state == "WANTED" && ((p.last_Req < req.Lamport) || (p.last_Req == req.Lamport && p.id < req.Id))) {

		log.Printf("(%v, %v) Recv | queueing request from %v\n", p.id, p.lamport, req.Id)
		//p.lamport++
		p.queue = append(p.queue, msg{id: req.Id, lamport: req.Lamport})
	} else {

		go func() {

			p.mutex.Lock()
			time.Sleep(10 * time.Millisecond)
			p.replies[req.Id] = false
			log.Printf("(%v, %v) Send | Allowing %v to enter critical section\n", p.id, p.lamport, req.Id)
			p.clients[req.Id].Reply(p.ctx, &message.Info{Id: p.id})
			p.mutex.Unlock()

		}()
	}

	p.mutex.Unlock()
	return &message.Empty{}, nil
}

func (p *peer) Reply(ctx context.Context, req *message.Info) (*message.Empty, error) {
	p.mutex.Lock()
	log.Printf("(%v, %v) Recv | Got reply from id %v\n", p.id, p.lamport, req.Id)
	p.replies[req.Id] = true

	if allTrue(p.replies) {
		p.state = "HELD"
		p.last_Req = math.MaxInt32
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
	p.last_Req = p.lamport
	log.Printf("(%v, %v) Send | Seeking critical section access", p.id, p.lamport)
	info := &message.Info{Id: p.id, Lamport: p.lamport}
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

func accessRessource(logFile *os.File, id int32, lamport int32, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, err := fmt.Fprintf(logFile, "[%s] (Lamport: %d) %v %s\n", timestamp, lamport, id, message)
	if err != nil {
		log.Printf("Failed to write to critical section log: %v", err)
	}
}

func setLog(port int32) (*os.File, *os.File) {

	filename := fmt.Sprintf("logs/peer(%v)-log.txt", port)
	if err := os.Truncate(filename, 0); err != nil {
		log.Printf("Failed to truncate: %v\n", err)
	}
	//peer
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	log.SetFlags(0)
	log.SetOutput(mw)

	//cs-log
	cslog, err := os.OpenFile("logs/cs-log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("Error opening critical section log file: %v", err)
	}

	return f, cslog
}
