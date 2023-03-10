# qperf

Benchmark QUIC bulk transfer throughput. Currently you can benchmark
the throughput achieved by the
[quic-go](https://github.com/lucas-clemente/quic-go) implementation of
the QUIC protocol in your environment.

## Protocol

On receiving a connection, the server opens a *uni*directional
**stream** to the client and writes *n* bytes of data to it. The
client must accept the unidirectional stream that the server
opens. The data written from the server to the client is made up of
random bytes that the client should discard as efficiently as
possible.

The reading peer can
[reset](https://www.rfc-editor.org/rfc/rfc9000.html#frame-reset-stream)
the unidirectional stream without signaling an error by sending
application error code 0 with the reset stream frame.

### Application Level Next Protocol Negotiation (ALPN)

Both the client and server must set the TLS Next Protocol value to: `quic-perf-test`.

## Installation

`go install -v github.com/marete/qperf@latest`

## Running

### On the server

`qperf -s -key ~/example.com.key -cert ~/example.com.crt -alsologtostderr`

### On the client

`qperf -c example.com:32850`

`qperf -c example.com:32850 -seconds 600`

You can skip validation of self-generated TLS certificates by invoking
the client with the `-insecure` flag.

By default the client will receive traffic for 30 seconds before
closing the connection and reporting statistics. This can be changed
with the `-seconds` flag.
