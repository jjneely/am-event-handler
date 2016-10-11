package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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

func TestExecution(t *testing.T) {
	// Holodeck safeties are off
	debug = false

	url := fmt.Sprintf("http://%s/", bind)
	buf := make([]byte, 4096)
	testcase := new(bytes.Buffer)

	// Remove our test marker, ignoring errors
	_ = os.Remove("testdata/unittest")

	json, err := os.Open("testdata/test5")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(testcase, json)
	json.Close()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(url, "application/foobar", testcase)
	if err != nil {
		t.Fatal(err)
	}
	n, err := resp.Body.Read(buf)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("Bad Status from test: %d  Body: %s", resp.StatusCode,
			string(buf[:n]))
	}

	// Does our test file exist?
	_, err = os.Stat("testdata/unittest")
	if os.IsNotExist(err) {
		t.Errorf("testdata/unittest should exist after event handled, but does not")
	} else if err != nil {
		t.Fatal(err)
	}
}

func TestTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	// Holodeck safeties are off
	debug = false

	url := fmt.Sprintf("http://%s/", bind)
	buf := make([]byte, 4096)
	testcase := new(bytes.Buffer)

	json, err := os.Open("testdata/test6")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(testcase, json)
	json.Close()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(url, "application/foobar", testcase)
	if err != nil {
		t.Fatal(err)
	}
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
