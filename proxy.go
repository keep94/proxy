package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/gorilla/context"
	"github.com/keep94/weblogs"
	"github.com/keep94/weblogs/loggers"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
)

var (
	fDestUrl = flag.String("dest_url", "", "DestinationUrl")
	fPort    = flag.String("port", ":8080", "Binding port")
)

type snapshot struct {
	*loggers.Snapshot
	Body   []byte
	Header http.Header
}

type capture struct {
	*loggers.Capture
	recorder *httptest.ResponseRecorder
}

func (c *capture) Write(buf []byte) (int, error) {
	numWritten, err := c.Capture.Write(buf)
	if err == nil {
		c.recorder.Write(buf[:numWritten])
	}
	return numWritten, err
}

func (c *capture) WriteHeader(status int) {
	c.recorder.HeaderMap = c.Capture.Header()
	c.Capture.WriteHeader(status)
}

type evesdropLogger struct {
}

func (e evesdropLogger) NewSnapshot(r *http.Request) weblogs.Snapshot {
	var result snapshot
	result.Snapshot = loggers.NewSnapshot(r)
	result.Body, _ = ioutil.ReadAll(r.Body)
	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(result.Body))
	result.Header = r.Header
	return &result
}

func (e evesdropLogger) NewCapture(w http.ResponseWriter) weblogs.Capture {
	return &capture{
		Capture:  &loggers.Capture{ResponseWriter: w},
		recorder: httptest.NewRecorder()}
}

func (e evesdropLogger) Log(w io.Writer, log *weblogs.LogRecord) {
	s := log.R.(*snapshot)
	c := log.W.(*capture)
	fmt.Fprintf(w, "%s - %s [%s] \"%s %s %s\" %d %d\n",
		loggers.StripPort(s.RemoteAddr),
		loggers.ApacheUser(s.URL.User),
		log.T.Format("02/Jan/2006:15:04:05 -0700"),
		s.Method,
		s.URL.RequestURI(),
		s.Proto,
		c.Status(),
		c.Size())
	fmt.Fprintln(w, "Header:")
	s.Header.Write(w)
	if len(s.Body) > 0 {
		fmt.Fprintln(w, "Body:")
		fmt.Fprintf(w, "%s\n", string(s.Body))
	}
	fmt.Fprintln(w, "Response header: ")
	c.recorder.HeaderMap.Write(w)
	responseBody := c.recorder.Body.Bytes()
	if len(responseBody) > 0 {
		fmt.Fprintln(w, "Response body:")
		fmt.Fprintf(w, "%s\n", string(responseBody))
	}
}

func main() {
	flag.Parse()
	url, _ := url.Parse(*fDestUrl)
	http.Handle("/", httputil.NewSingleHostReverseProxy(url))

	defaultHandler := context.ClearHandler(
		weblogs.HandlerWithOptions(
			http.DefaultServeMux,
			&weblogs.Options{Logger: evesdropLogger{}}))
	if err := http.ListenAndServe(*fPort, defaultHandler); err != nil {
		fmt.Println(err)
	}
}
