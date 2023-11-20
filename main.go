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

type peer struct {
	message.UnimplementedServiceServer
	id      int32
	mutex   sync.Mutex
	replies int32
	held    chan bool
	state   string
	lamport int32
	// a slice
	queue []message.Info
	// Update on this channel, when when messages should be dequeued
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
		replies: 0,
		held:    make(chan bool),
		ctx:     ctx,
		state:   "RELEASED",
		lamport: 0,
		reply:   make(chan bool),
	}

	// Create listener tcp on port ownPort
	list, err := net.Listen("tcp", fmt.Sprintf(":%v", ownPort))
	if err != nil {
		log.Fatalf("Could not listen on port: %v", err)
	}

	f := setLog(ownPort)
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
				p.lamport = max(msg.Lamport, p.lamport) + 1
				log.Printf("(%v, %v) Send | Allowing %v to enter critical section (queue release) \n", p.id, p.lamport, msg.Id)
				p.clients[msg.Id].Reply(p.ctx, &message.Info{Id: p.id})
			}
			p.queue = nil
			p.state = "RELEASED"
			p.mutex.Unlock()
		}
	}()

	rand.Seed(time.Now().UnixNano() / int64(ownPort))
	// for {
	// 	// 1/100 chance to want access
	// 	if rand.Intn(100) == 1 {
	// 		p.mutex.Lock()
	// 		p.state = "WANTED"
	// 		p.mutex.Unlock()
	// 		p.enter()
	// 		// wait for state to be held
	// 		<-p.held
	// 		p.lamport++
	// 		log.Printf("(%v, %v) Internal | Entered critical section\n", p.id, p.lamport)
	// 		time.Sleep(5 * time.Second)
	// 		p.lamport++
	// 		log.Printf("(%v, %v) Internal | Leaving critical section\n", p.id, p.lamport)
	// 		p.reply <- true
	// 	}
	// 	time.Sleep(50 * time.Millisecond)
	// }
	for {
		p.mutex.Lock()
		p.state = "WANTED"
		p.mutex.Unlock()
		p.enter()
		// wait for state to be held
		<-p.held
		p.lamport++
		log.Printf("(%v, %v) Internal | Entered critical section\n", p.id, p.lamport)
		time.Sleep(5 * time.Second)
		p.lamport++
		log.Printf("(%v, %v) Internal | Leaving critical section\n", p.id, p.lamport)
		p.reply <- true
	}
}

func (p *peer) Request(ctx context.Context, req *message.Info) (*message.Empty, error) {
	p.mutex.Lock()
	p.lamport = max(req.Lamport, p.lamport) + 1
	if p.state == "HELD" || (p.state == "WANTED" && ((p.lamport < req.Lamport) || (p.lamport == req.Lamport && p.id < req.Id))) {
		log.Printf("(%v, %v) Recv | queueing request from %v\n", p.id, p.lamport, req.Id)
		p.queue = append(p.queue, *req)
	} else {

		go func() {
			time.Sleep(10 * time.Millisecond)
			p.lamport++
			log.Printf("(%v, %v) Send | Allowing %v to enter critical section\n", p.id, p.lamport, req.Id)
			p.clients[req.Id].Reply(p.ctx, &message.Info{Id: p.id, Lamport: p.lamport})
		}()
	}

	p.mutex.Unlock()
	return &message.Empty{}, nil
}

func (p *peer) Reply(ctx context.Context, req *message.Info) (*message.Empty, error) {
	p.lamport = max(p.lamport, req.Lamport) + 1
	log.Printf("(%v, %v) Recv | Got reply from id %v\n", p.id, p.lamport, req.Id)
	p.mutex.Lock()
	p.replies++
	if p.replies >= 2 {
		p.state = "HELD"
		p.replies = 0
		p.mutex.Unlock()
		p.held <- true
	} else {
		p.mutex.Unlock()
	}
	return &message.Empty{}, nil
}

func (p *peer) enter() {

	for id, client := range p.clients {
		p.lamport++
		log.Printf("(%v, %v) Send | Seeking critical section access", p.id, p.lamport)
		info := &message.Info{Id: p.id, Lamport: p.lamport}
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

func setLog(port int32) *os.File {
	// Clears the log.txt file when a new server is started
	filename := fmt.Sprintf("peer(%v)-log.txt", port)
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
	return f
}
