package main

import (
	"context"
	"crypto/tls"
	"flag"
	"io"
	"net"
	"os"

	"github.com/golang/glog"
	"github.com/lucas-clemente/quic-go"
)

var (
	key    = flag.String("key", "", "path to the tls private key file")
	cert   = flag.String("cert", "", "path to the tls certificate file")
	addr   = flag.String("addr", ":32850", "listen on this address")
	serve  = flag.Bool("s", false, "run as a server")
	client = flag.String("c", "localhost:32850", "run as a client to specified remote")
)

var data [1 << 16]byte

const alpnNextProto = "quic-perf-test"

const totalData = 10 << 30

func serverMain(ctx context.Context) {
	rf, err := os.Open("/dev/urandom")
	if err != nil {
		glog.Exitf("Fatal error opening source of random data: %v", err)
	}
	_, err = io.ReadFull(rf, data[:])
	if err != nil {
		glog.Exitf("Couldn't read all the random bytes we wanted: %v", err)
	}
	rf.Close()

	cert, err := tls.LoadX509KeyPair(*cert, *key)
	if err != nil {
		glog.Exitf("Fatal error loading TLS key pair: %v", err)
	}

	c := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{alpnNextProto},
	}

	l, err := quic.ListenAddr(*addr, c, nil)
	if err != nil {
		glog.Exitf("Fatal error listening on %s: %v", *addr, err)
	}

	glog.Infof("Listening on address %v", *addr)
	defer l.Close()

	for {
		conn, err := l.Accept(ctx)
		if err != nil {
			glog.Errorf("Error accepting connection: %v", err)
			continue
		}
		glog.Infof("Accepted connection from %s", conn.RemoteAddr())

		go func(conn quic.Connection) {
			nBytes := uint64(0)
			defer func() {
				glog.Infof("Wrote %d bytes to client: %s", nBytes, conn.RemoteAddr())
			}()

			glog.Infof("Opening Unidirectional stream connection to client: %s", conn.RemoteAddr())
			s, err := conn.OpenUniStreamSync(ctx)
			if err != nil {
				glog.Errorf("Error opening unidirectional stream to  client: %s: %v", conn.RemoteAddr(), err)
				return
			}
			defer s.Close()
			for nBytes <= totalData {
				n, err := s.Write(data[:])
				if err != nil {
					glog.Errorf("Error writing to client: %s: %v", conn.RemoteAddr(),
						err)
					return
				}
				nBytes += uint64(n)
			}
		}(conn)
	}

}

func clientMain(ctx context.Context) {
	host, _, err := net.SplitHostPort(*client)
	if err != nil {
		glog.Exitf("Fatal error parsing server address: %v", err)
	}

	tlsConfig := &tls.Config{
		NextProtos: []string{alpnNextProto},
		ServerName: host,
	}

	conn, err := quic.DialAddr(*client, tlsConfig, nil)
	if err != nil {
		glog.Exitf("Fatal error establishing connection: %v", err)
	}

	s, err := conn.AcceptUniStream(ctx)
	if err != nil {
		glog.Errorf("Fatal error opening stream to %s: %v", conn.RemoteAddr(), err)
	}

	_, err = io.Copy(io.Discard, s)
	if err != nil {
		glog.Exitf("Fatal error reading from stream: %v", err)
	}
}

func main() {
	flag.Parse()

	if *serve {
		serverMain(context.Background())
	}

	clientMain(context.Background())
}
