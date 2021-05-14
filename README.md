# KeepAlive-module
A simple UDP based KeepAlive module is developed in GoLang.

Software Version:
------------------
go version go1.15.6 linux/amd64

Description:
------------
The KeepAlive detects situations where connection between network elements is alive or not. A simple approach to identify state of connection by sending probe messages to see if the other side responds. If it does not respond to a certain number of probes within a certain amount of time, then it assumes the connection is dead. 

KeepAlive server creates, listens on provided UDP IP, port combination. Also, a go routine initialized to process all keepalive messages received from peers. Peer client nodes request a UDP connection to server and sends key parameters {peer name, keepalive interval and keepalive probes}. Server on receiving a peer connection request, a new entry is created from map, update the peer details in the map followed by go routine initialized to send keepalive messages towards peers on every interval ticker expiry. At any point when the keepalive probes from server reaches max probes configured for a peer, keepalive connection is declared down. Refer to log.txt file for test logs for both server and clients.

Test Execution:
----------
Server:
```
[root@centos keepalive-module]# go build -race ka-server.go
[root@centos keepalive-module]# ./ka-server <local_address>
````
Client:
```
[root@centos Linux]# go build -race ka-client.go
[root@centos Linux]# ./ka-client <local address> <remote address> <peer identifier> <interval> <probes>
```
![KeepAlive wireshark capture](https://user-images.githubusercontent.com/6613674/118281754-31c15480-b4eb-11eb-88bd-3ef07ebfc581.PNG)

Notes: Above capture is a simple test which contains keepalive messages between server and clients. Frame 32 is initial message from peer to server in order to register for keepalive mechanism. Subsequent 2 frames from server every keepalive interval every 30 secs has corresponding responses from peer as shown. Now the peer is closed abruptly, where we can see 5 keepalive messages from server are not reached at peer node. When server did not receive responses from peer for configured keepalive probe count, peer will be declared dead and server stops sending any further keepalive messages to the peer.     
