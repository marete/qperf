package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/logging"
	"github.com/lucas-clemente/quic-go/qlog"
	"golang.org/x/time/rate"
)

var (
	key      = flag.String("key", "", "path to the tls private key file")
	cert     = flag.String("cert", "", "path to the tls certificate file")
	addr     = flag.String("addr", ":32850", "listen on this address")
	serve    = flag.Bool("s", false, "run as a server")
	client   = flag.String("c", "localhost:32850", "run as a client to specified remote")
	insecure = flag.Bool("insecure", false, "don't verify TLS certificate details")
	qlogDir  = flag.String("qlog-dest-dir", "", "activate qlog writing and write the qlogs in this directory")
	dLimit   = flag.String("b", "", "limit download bitrate to this bits/sec (a [KMG] suffix is allowed e.g. 1G to limit download speed to 1 Gigabits/s")
)

var data [1 << 16]byte

const alpnNextProto = "quic-perf-test"

const totalData = 10 << 30

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

func limiterFromString(l string) (*rate.Limiter, error) {
	if l == "" {
		return nil, nil
	}

	var limit float64
	limit, err := strconv.ParseFloat(l, 64)
	if err == nil {
		return rate.NewLimiter(rate.Limit(1.25*limit/8), readChunkSize), nil
	}

	_, err = fmt.Sscanf(l, "%fK", &limit)
	if err == nil {
		return rate.NewLimiter(rate.Limit(1.25*1e3*limit/8), readChunkSize), nil
	}

	_, err = fmt.Sscanf(l, "%fM", &limit)
	if err == nil {
		return rate.NewLimiter(rate.Limit(1.25*1e6*limit/8), readChunkSize), nil
	}

	_, err = fmt.Sscanf(l, "%fG", &limit)
	if err == nil {
		return rate.NewLimiter(rate.Limit(1.25*1e9*limit/8), readChunkSize), nil
	}

	_, err = fmt.Sscanf(l, "%fT", &limit)
	if err == nil {
		return rate.NewLimiter(rate.Limit(1.25*1e12*limit/8), readChunkSize), nil
	}

	return nil, errors.New("faild to parse download limit")
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

	conn, err := quic.DialAddr(*client, tlsConfig, &qconf)
	if err != nil {
		glog.Exitf("Fatal error establishing connection: %v", err)
	}

	s, err := conn.AcceptUniStream(ctx)
	if err != nil {
		glog.Errorf("Fatal error opening stream to %s: %v", conn.RemoteAddr(), err)
	}

	limiter := rate.NewLimiter(rate.Inf, 0)
	userLimiter, err := limiterFromString(*dLimit)
	if err != nil {
		glog.Exitf("Fatal error parsing the `-b' option. Please invoke command with --help")
	}
	if userLimiter != nil {
		glog.Infof("limiter: limit=%f, burst=%d", userLimiter.Limit(), userLimiter.Burst())
		limiter = userLimiter
	}

	glog.Infof("limiter: limit=%f, burst=%d", limiter.Limit(), limiter.Burst())

	var discard [readChunkSize]byte
	n := uint64(0)
	start := time.Now()
	for {
		err = limiter.WaitN(ctx, len(discard))
		if err != nil {
			glog.Exitf("Fatal error waiting for tokens from limter: %v", err)
		}

		i, err := s.Read(discard[:])
		n += uint64(i)
		if err != nil {
			if err == io.EOF {
				break
			}
			glog.Exitf("Fatal error reading from stream: %v", err)
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
