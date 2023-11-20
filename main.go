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
	replies int32
	held    chan bool
	state   string
	lamport int32
	// some gRPC calls use empty, prevent making a new one each time
	empty message.Empty
	// same as above, except for replies
	idmsg message.Id
	// queued "messages" get appended here, FIFO, in actuality we just store the id of the
	// peer instance we wish to 'gRPC.Reply' to, and the lamport timestamp for updating own lamport
	queue []msg
	// fire update on this channel, when we need to send messages in our queue
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
		empty:   message.Empty{},
		idmsg:   message.Id{Id: ownPort},
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
	// as seen from log message above - this is because if we do not sleep, then the first (or second) client will cause
	// invalid control flow in the different gRPC functions on last client, since it might not have an active connection to the one dialing it yet
	// - the simplest solution, is to just wait a little, rather than inquiring each client about whether or not it has N-clients dialed, like ourselves
	time.Sleep(5 * time.Second)

	// start our queue loop, it will (when told to) - send messages that have been delayed
	go func() {
		for {
			// wait for update
			<-p.reply
			p.mutex.Lock()
			for _, msg := range p.queue {
				// update our lamport to the max found value in queue
				p.lamport = max(msg.lamport, p.lamport)
				log.Printf("Queue | Giving permission to enter critical section to %v\n", msg.id)
				p.clients[msg.id].Reply(p.ctx, &p.idmsg)
			}
			// see previous comment, increment highest found
			p.lamport++
			p.queue = nil
			p.state = "RELEASED"
			p.mutex.Unlock()
		}
	}()

	// not cryptographically secure, however, it does not need to be - we are only using it so that
	// the programs dont request access at the same time :)
	rand.Seed(time.Now().UnixNano() / int64(ownPort))
	for {
		// 1/100 chance
		if rand.Intn(100) == 1 {
			p.mutex.Lock()
			p.state = "WANTED"
			p.mutex.Unlock()
			p.enter()
			// wait for state to be held
			<-p.held
			log.Printf("(%v, %v) + Entered critical section\n", p.id, p.lamport)
			time.Sleep(5 * time.Second)
			log.Printf("(%v, %v) - Leaving critical section\n", p.id, p.lamport)
			// fire all of our queued replies, this also sets our state back to released
			p.reply <- true
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (p *peer) Request(ctx context.Context, req *message.Info) (*message.Empty, error) {
	p.mutex.Lock()
	if p.state == "HELD" || (p.state == "WANTED" && ((p.lamport < req.Lamport) || (p.lamport == req.Lamport && p.id < req.Id))) {
		log.Printf("Request | Received request from %v, appending...\n", req.Id)
		p.queue = append(p.queue, msg{id: req.Id, lamport: req.Lamport})
	} else {
		p.lamport = max(req.Lamport, p.lamport) + 1
		// we need the reply to arrive later than request finishing up, which is messy
		// reply/request could instead have been combined, and then depending on value - the peer
		// would know whether or not a request succeeded... however, this simplifies logic, since it allows
		// to *only* count amount of replies we've received to know whether or not we can enter critical section
		go func() {
			time.Sleep(10 * time.Millisecond)
			log.Printf("(%v, %v) Request | Allowing %v to enter critical section\n", p.id, p.lamport, req.Id)
			p.clients[req.Id].Reply(p.ctx, &p.idmsg)
		}()
	}

	p.mutex.Unlock()
	return &p.empty, nil
}

func (p *peer) Reply(ctx context.Context, req *message.Id) (*message.Empty, error) {
	log.Printf("(%v, %v) Reply | Got reply from id %v\n", p.id, p.lamport, req.Id)
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

	return &p.empty, nil
}

func (p *peer) enter() {
	log.Printf("Enter | Seeking critical section access")
	info := &message.Info{Id: p.id, Lamport: p.lamport}
	for id, client := range p.clients {
		_, err := client.Request(p.ctx, info)
		if err != nil {
			log.Printf("something went wrong with id %v\n", id)
		}
		//log.Printf("Enter | Requested from %v\n", id)
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
