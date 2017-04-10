package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var testdata = map[string]int{
	"testdata/test1": 200, // No handler in annotation
	"testdata/test2": 200, // No handler in annotation
	"testdata/test3": 200, // Command not found on system (debug mode)
	"testdata/test4": 200, // Runs command on local system with templating
	"testdata/test9": 200, // Templating/quoting tests
}

var bind = "127.0.0.1:4242"

func init() {
	var err error

	// load test configuration into global config variable
	debug = true
	verbose = true
	timeout = time.Second * 15
	config, err = loadConfiguration("testdata/config.yaml")
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

func postHelper(filename string) (*http.Response, error) {
	url := fmt.Sprintf("http://%s/", bind)
	testcase := new(bytes.Buffer)

	json, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(testcase, json)
	json.Close()
	if err != nil {
		return nil, err
	}

	return http.Post(url, "application/foobar", testcase)
}

func executeTest(t *testing.T, testcase, flagFile string) {
	// Holodeck safeties are off
	debug = false

	// Remove our test marker, ignoring errors
	_ = os.Remove(flagFile)

	resp, err := postHelper(testcase)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("Bad Status from test: %d  Body: %s", resp.StatusCode,
			string(buf[:n]))
	}

	// Does our test file exist?
	_, err = os.Stat(flagFile)
	if os.IsNotExist(err) {
		t.Errorf("%s should exist after event handled, but does not", flagFile)
	} else if err != nil {
		t.Fatal(err)
	}

	_ = os.Remove(flagFile)
}

func TestExecution(t *testing.T) {
	executeTest(t, "testdata/test5", "testdata/unittest")
}

func TestDefaultHandler(t *testing.T) {
	config.Handlers["default"] = struct {
		Command string
		Status  string
	}{
		Command: "/bin/bash -c \"touch testdata/testDefault\"",
		Status:  "*",
	}
	executeTest(t, "testdata/test1", "testdata/testDefault")
	delete(config.Handlers, "default")
}

func TestAllHandler(t *testing.T) {
	config.Handlers["all"] = struct {
		Command string
		Status  string
	}{
		Command: "/bin/bash -c \"touch testdata/testAll\"",
		Status:  "*",
	}
	executeTest(t, "testdata/test1", "testdata/testAll")
	delete(config.Handlers, "all")
}

func TestTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	// Holodeck safeties are off
	debug = false

	resp, err := postHelper("testdata/test6")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("Bad Status from test: %d  Body: %s", resp.StatusCode,
			string(buf[:n]))
	}

	t.Logf("Response body: %s", string(buf[:n]))
	if !strings.Contains(string(buf[:n]), "timed out") {
		t.Errorf("Timeout test did not return a timeout error")
	}
}

func TestJson(t *testing.T) {
	// Holodeck safeties are off
	debug = false

	resp, err := postHelper("testdata/test7")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("Bad Status from test: %d  Body: %s", resp.StatusCode,
			string(buf[:n]))
	}

	t.Logf("Response body: %s", string(buf[:n]))
	alert := new(Alert)
	err = json.Unmarshal(buf[:n], alert)
	if err != nil {
		t.Fatalf("JSON unmarshalling failed")
	}

	// XXX: Does alert match what we expect?
}

func TestLargeSize(t *testing.T) {
	var body []byte

	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	// Holodeck safeties are on
	debug = true

	resp, err := postHelper("testdata/test8")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n := 0
	for err == nil {
		n, err = resp.Body.Read(buf)
		t.Logf("Read %d bytes.  IO error: %v", n, err)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
	}
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Bad Status from test: %d", resp.StatusCode)
	}
}
