package main

import (
	"os"
	"fmt"
	"net"
	"strings"
	"strconv"
)

func errorCheck(err error) {
	if err != nil {
		fmt.Println("Error: " , err)
	}
}

func main() {
	if len(os.Args) != 6 {
		fmt.Println("Usage:", os.Args[0], "local address", "remote address", "peer identifier", "interval", "probes")
		return
	}

	local_ip_port := fmt.Sprintf("%s:%d", os.Args[1], 5002)
	svr1_ip_port := fmt.Sprintf("%s:%d", os.Args[2], 5001)
	svr2_ip_port := fmt.Sprintf("%s:%d", os.Args[2], 5002)

	peerId := os.Args[3]
	keepalive_intvl, err := strconv.Atoi(os.Args[4])
	keepalive_probes, err := strconv.Atoi(os.Args[5])

	if (keepalive_intvl <= 0 || keepalive_probes <= 0) {
		fmt.Println("Invalid keepalive interval or probes")
		return
	}

	svraddr1, err := net.ResolveUDPAddr("udp4", svr1_ip_port)
	errorCheck(err)

	svraddr2, err := net.ResolveUDPAddr("udp4", svr2_ip_port)
	errorCheck(err)

	localaddr, err := net.ResolveUDPAddr("udp4", local_ip_port)
	errorCheck(err)

	Conn1, err := net.DialUDP("udp4", nil, svraddr1)
	errorCheck(err)
	defer Conn1.Close()

	/* init msg contains peer name, ka interval and probe count */
	first_msg := fmt.Sprintf("%s:%d:%d", peerId, keepalive_intvl, keepalive_probes)

	_, err = Conn1.Write([]byte(first_msg))
	if err != nil {
		fmt.Println(first_msg, err)
	}

	Conn2, err := net.DialUDP("udp4", localaddr, svraddr2)
	errorCheck(err)
	defer Conn2.Close()

	for {
		buffer := make([]byte, 256)
		n, err := Conn2.Read(buffer)
		if err != nil {
			fmt.Println(err)
			continue
		}

		fmt.Printf("Server reply: %s\n", string(buffer[0:n]))

		if len(buffer[0:n]) > 0 {
			split := strings.Split(string(buffer), ":")
			if len(split) == 1 {
				fmt.Printf("Invalid keepalive format %s received", buffer)
				continue
			}
			rsp_msg := fmt.Sprintf("%s:%s", peerId, split[1])

			_, err = Conn2.Write([]byte(rsp_msg))
			if err != nil {
				fmt.Println(rsp_msg, err)
			}
		}
    	}
}
