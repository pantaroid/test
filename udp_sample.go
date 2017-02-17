package main

import (
  "net"
  "fmt"
)

func main() {
  remote, err := net.ResolveUDPAddr("udp", "localhost:51702")
  if err != nil {
    fmt.Printf("%v\n", err)
  }
  conn, err := net.DialUDP("udp", nil, remote)
  if err != nil {
    fmt.Printf("%v\n", err)
  }
  //conn.SetDeadline(time.Now().Add(5 * time.Second))
  defer conn.Close()

  conn.Write([]byte("A:123:abc:!!@!"))
}

