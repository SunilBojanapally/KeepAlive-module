package main

import (
	"os"
	"fmt"
	"net"
	"sync"
	"time"
	"bytes"
	"strings"
	"strconv"
	"syscall"
	"os/signal"
	)

const (
	DEFAULT_PEER_INIT_CONN_PORT = 5001
	DEFAULT_PEER_KEEPALIVE_PORT = 5002
)

type udpServer struct {
	Conn *net.UDPConn
}

type ka_params struct {
	peerId              string    /* unique peer identifier */
	keepalive_intvl     int       /* the interval between subsequential keepalive probes */
	keepalive_probes    int       /* the number of unacknowledged probes */
}

type ka_peer struct {
	Mu                  sync.Mutex
	Stop                chan bool
	Peer                ka_params
	token               int
	Isalive             bool
	noRspProbesCnt      int
	KeepAliveTicker     *time.Ticker
}

var local_addr string
var udpserver *udpServer = nil

var ka_map = struct {
	sync.RWMutex
	peer map[string]*ka_peer
}{peer: make(map[string]*ka_peer)}

func bindAddress(port int) *net.UDPConn {
	addr := net.UDPAddr{
		Port: port,
		IP:   net.ParseIP(local_addr),
	}
	laddr, err := net.ListenUDP("udp4", &addr)
	if err != nil {
		fmt.Println("Listen on UDP failed ", err)
		return nil
	}
	return laddr
}

func listenForPeers() {
	fmt.Println("Waiting for peers to connect...")
	var peerId string
	var peerKeepAliveIntvl int
	var peerKeepAliveProbes int

	peer := bindAddress(DEFAULT_PEER_INIT_CONN_PORT)

	for {
		buf := make([]byte, 256)
		n, peer_addr, err := peer.ReadFromUDP(buf)
		if err != nil {
			fmt.Printf("Error %v\n", err)
			continue
		}

		if len(buf[0:n]) > 0 {
			n = bytes.Index(buf, []byte{0})
			raw := make([]byte, n)
			copy(raw, buf)
			split := bytes.Split(raw, []byte(":"))
			if len(split) == 1 {
				fmt.Printf("Invalid peer init format: %s", raw)
				continue
			}
			peerId = string(split[0])
			peerKeepAliveIntvl, _ = strconv.Atoi(string(split[1]))
			peerKeepAliveProbes, _ = strconv.Atoi(string(split[2]))
		} else {
			continue
		}

		ka_map.RLock()
		if _, ok := ka_map.peer[peerId]; ok {
			ka_map.RUnlock()
			fmt.Printf("Bailing out, peer: %s already configured\n", peerId)
			continue
		} else {
			ka_map.RUnlock()
		}

		addr := net.UDPAddr{
			Port: DEFAULT_PEER_KEEPALIVE_PORT,
			IP:   peer_addr.IP,
		}

		go add_peer(peerId, peerKeepAliveIntvl, peerKeepAliveProbes, &addr)
	}
}

func processPeerResponse() {

	buffer := make([]byte, 256)
	for {
		// format of message "peerId:ka_token"
		n, err := udpserver.Conn.Read(buffer)
		if err != nil {
			fmt.Println("processPeerRsponse read failure ", err)
			continue
		}

		if len(buffer[0:n]) > 0 {
			// Decode peer entry from rcvd buff using peerId
			split := strings.Split(string(buffer), ":")
			if len(split) == 1 {
				fmt.Printf("Invalid KeepAlive format rcvd: %s", buffer)
				continue
			}

			peerId := split[0]
			//Ignoring received token from peer
			//token := split[1]

			ka_map.RLock()
			if p, ok := ka_map.peer[peerId]; ok {
				ka_map.RUnlock()
				if !p.Isalive {
					fmt.Printf("KeepAlive received from %s, but it is not live", peerId)
					return
				}

				p.Mu.Lock()
				p.noRspProbesCnt = 0
				p.Mu.Unlock()
				fmt.Println("noRspProbesCnt set to zero for peer: ", p.Peer.peerId)
			} else {
				ka_map.RUnlock()
				// This could be due to late keepalive received
				fmt.Printf("Received delayed KeepAlive from peer: %s, ignoring\n", peerId)
			}
		}
	}
}

func processRequestToPeer(peerId string, peer_addr *net.UDPAddr) {

	ka_map.RLock()
	p := ka_map.peer[peerId]
	ka_map.RUnlock()

	for {
		select {
		case <-p.KeepAliveTicker.C:
			if !p.Isalive {
				continue
			}

			p.token++

			//format of keepalive message "keepalive:token"
			peerMsg := fmt.Sprintf("%s:%d", "keepalive", p.token)

			_, err := udpserver.Conn.WriteToUDP([]byte(peerMsg), peer_addr)
			if err != nil {
				fmt.Println("processRequestToPeer: Couldn't send response ", err)
			}

			//Reset NoRsp Probes count
			p.Mu.Lock()
			p.noRspProbesCnt++
			fmt.Printf("sent KeepAlive msg: %s to peer: %s, current noRspProbesCnt: %d\n", peerMsg, peerId, p.noRspProbesCnt)

			if p.noRspProbesCnt >= p.Peer.keepalive_probes {
				p.Mu.Unlock()
				fmt.Println("KeepAlive status down for peer: ", peerId)
				p.Isalive = false
				p.Stop <- true
				return
			} else {
				p.Mu.Unlock()
			}
		}
	}
}

func add_peer(peerId string, peerKeepAliveIntvl int, peerKeepAliveProbes int, peer_addr *net.UDPAddr) {
	fmt.Printf("Peer: %s connected with address: %s keepalive intvl: %d keepalive probes: %d\n",
		peerId, peer_addr, peerKeepAliveIntvl, peerKeepAliveProbes)

	p := newKeepAlivePeer(peerKeepAliveIntvl)
	p.Isalive = true
	p.Peer.peerId = peerId
	p.Peer.keepalive_probes = peerKeepAliveProbes
	ka_map.Lock()
	ka_map.peer[p.Peer.peerId] = p
	ka_map.Unlock()

	go processRequestToPeer(p.Peer.peerId, peer_addr)

	<-p.Stop
	p.Isalive = false
	p.KeepAliveTicker.Stop()
	ka_map.Lock()
	delete(ka_map.peer, p.Peer.peerId)
	ka_map.Unlock()
	peer_addr = nil
	p = nil
}

func newKeepAlivePeer(peerKeepAliveIntvl int) *ka_peer {
	return &ka_peer {
		Stop:                make(chan bool),
		Peer:                ka_params{peerId: "", keepalive_intvl: 60, keepalive_probes: 5},
		token:               0,
		Isalive:             false,
		noRspProbesCnt:      0,
		KeepAliveTicker:     time.NewTicker(time.Duration(peerKeepAliveIntvl) *time.Second),
	}
}

func newUdpServer() *udpServer {
	return &udpServer{
		Conn: bindAddress(DEFAULT_PEER_KEEPALIVE_PORT),
	}
}

func startUdpServer() {
	udpserver = newUdpServer();
	fmt.Println("Listening for peer nodes on:", udpserver.Conn.LocalAddr())
	defer udpserver.Conn.LocalAddr()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage:", os.Args[0], "local_address")
		return
	}

	local_addr = os.Args[1]
	/* Create a UDP server connection to communicate to external world */
	startUdpServer()
	/* Create one go routine to process all peer's keepalive responses */
	go processPeerResponse()
	/* Listen for peer's initialization before keepalive start */
	go listenForPeers()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done

	// Clean up
	ka_map.RLock()
	for key, p := range ka_map.peer {
		fmt.Printf("freeing peer: %s entry\n", key)
		p.Isalive = false
		p.Stop <- true
	}
	ka_map.RUnlock()
}
