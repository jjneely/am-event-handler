package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

var testdata = map[string]int{
	"testdata/test1": 400, // No handler in annotation
	"testdata/test2": 400, // No handler in annotation
	"testdata/test3": 200, // Command not found on system
	"testdata/test4": 200, // Runs command on local system with templating
}

var bind = "127.0.0.1:4242"

func init() {
	var err error

	// load test configuration into global config variable
	debug = true
	config, err = loadConfiguration("./testdata/config.yaml")
	if err != nil {
		panic("Could not load test configuration: " + err.Error())
	}

	go run(bind)
	time.Sleep(1 * time.Second)
}

func TestREST(t *testing.T) {
	url := fmt.Sprintf("http://%s/", bind)
	buf := make([]byte, 4096)
	for k, v := range testdata {
		t.Logf("Testing %s", k)
		testcase := new(bytes.Buffer)
		json, err := os.Open(k)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(testcase, json)
		json.Close()
		if err != nil {
			t.Fatal(err)
		}

		resp, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 400 {
			// GETs are always Bad Requests
			t.Errorf("GET test returned status code %d", resp.StatusCode)
		}
		resp.Body.Close()
		resp, err = http.Post(url, "application/foobar", testcase)
		if err != nil {
			t.Fatal(err)
		}
		n, err := resp.Body.Read(buf)
		resp.Body.Close()
		if resp.StatusCode != v {
			t.Errorf("Bad Status from test: %d  Body: %s", resp.StatusCode,
				string(buf[:n]))
		}
	}
}
