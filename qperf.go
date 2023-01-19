package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/glog"
	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/logging"
	"github.com/lucas-clemente/quic-go/qlog"
)

var (
	key            = flag.String("key", "", "path to the tls private key file")
	cert           = flag.String("cert", "", "path to the tls certificate file")
	addr           = flag.String("addr", ":32850", "listen on this address")
	serve          = flag.Bool("s", false, "run as a server")
	client         = flag.String("c", "localhost:32850", "run as a client to specified remote")
	insecure       = flag.Bool("insecure", false, "don't verify TLS certificate details")
	qlogDir        = flag.String("qlog-dest-dir", "", "activate qlog writing and write the qlogs in this directory")
	durationInSecs = flag.Int64("seconds", 30, "run the test for this number of seconds.")
)

var data [1 << 16]byte

const alpnNextProto = "quic-perf-test"

const readChunkSize = 8 << 10

type bufferedWriteCloser struct {
	*bufio.Writer
	io.Closer
}

// NewBufferedWriteCloser creates an io.WriteCloser from a bufio.Writer and an io.Closer
func newBufferedWriteCloser(writer *bufio.Writer, closer io.Closer) io.WriteCloser {
	return &bufferedWriteCloser{
		Writer: writer,
		Closer: closer,
	}
}

func (h bufferedWriteCloser) Close() error {
	if err := h.Writer.Flush(); err != nil {
		return err
	}
	return h.Closer.Close()
}

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
		Certificates:       []tls.Certificate{cert},
		NextProtos:         []string{alpnNextProto},
		InsecureSkipVerify: *insecure,
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

			for {
				n, err := s.Write(data[:])
				if err != nil {
					if e, ok := err.(*quic.ApplicationError); ok {
						if e.ErrorCode == quic.ApplicationErrorCode(0) {
							return
						}
					}
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

	var qconf quic.Config
	qconf.EnableDatagrams = true

	if *qlogDir != "" {
		glog.Infof("Qlog logging enabled, will write qlog files to this dir: %s", *qlogDir)
		qconf.Tracer = qlog.NewTracer(func(_ logging.Perspective, connID []byte) io.WriteCloser {
			baseName := fmt.Sprintf("client_%x.qlog", connID)
			fname := filepath.Join(*qlogDir, baseName)
			f, err := os.Create(fname)
			if err != nil {
				glog.Fatalf("Qlog: Failed to create file: %s: %v", fname, err)
			}
			glog.Infof("Created new qlog file: %s", fname)
			return newBufferedWriteCloser(bufio.NewWriter(f), f)
		})

	}

	conn, err := quic.DialAddrContext(ctx, *client, tlsConfig, &qconf)
	if err != nil {
		glog.Exitf("Fatal error establishing connection: %v", err)
	}
	defer conn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "done")

	s, err := conn.AcceptUniStream(ctx)
	if err != nil {
		glog.Exitf("Fatal error accepting unidirectional stream from %s: %v", conn.RemoteAddr(), err)
	}
	defer s.CancelRead(quic.StreamErrorCode(quic.NoError))

	err = s.SetReadDeadline(time.Now().Add(time.Duration(*durationInSecs) * time.Second))
	if err != nil {
		glog.Exitf("Fatal error setting a read deadline on unidirectional stream: %v", err)
	}

	doneCh := ctx.Done()

	var discard [readChunkSize]byte
	n := uint64(0)
	start := time.Now()
	for {
		if doneCh != nil {
			select {
			case <-doneCh:
				return
			default:
			}
		}

		i, err := s.Read(discard[:])
		n += uint64(i)
		if err != nil {
			if err == io.EOF {
				break
			}

			if e, ok := err.(net.Error); ok {
				if e.Timeout() {
					break
				}
			}

			glog.Errorf("Error reading from stream: %v", err)
			break
		}
	}
	dur := time.Since(start)
	durS := float64(dur) / 1e9
	fmt.Printf("Received: %d bytes in %.3f seconds (%.3f Kbits/s)\n",
		n,
		durS,
		((float64(n)/1e3)*8)/float64(durS))

}

func main() {
	flag.Parse()

	if *serve {
		serverMain(context.Background())
	}

	clientMain(context.Background())
}
