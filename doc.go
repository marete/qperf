// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

/*
	   qperf is a tool to benchmark QUIC bulk transfer throughput and latency.

	   qperf runs either as a server (option -s) or as a client (option -c).

	   qperf is also a protocol, which is described in the [README]: https://github.com/marete/qperf#protocol

	   Usage of qperf:

	       qperf [flags]

	   The flags are:

	       -addr string
		     listen on this address (default ":32850")
	       -alsologtostderr
		     log to standard error as well as files
	       -c string
		     run as a client to specified remote (default "localhost:32850")
	       -cert string
		     path to the tls certificate file
	       -insecure
		     don't verify TLS certificate details
	       -key string
		     path to the tls private key file
	       -log_backtrace_at value
		     when logging hits line file:N, emit a stack trace
	       -log_dir string
		     If non-empty, write log files in this directory
	       -logtostderr
		     log to standard error instead of files
	       -qlog-dest-dir string
		     activate qlog writing and write the qlogs in this directory
	       -s	run as a server
	       -seconds int
		     run the test for this number of seconds. (default 30)
	       -stderrthreshold value
		     logs at or above this threshold go to stderr
	       -v value
		     log level for V logs
	       -vmodule value
		     comma-separated list of pattern=N settings for file-filtered logging
*/
package main
