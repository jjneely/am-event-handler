package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	// 4KiB buffer for the JSON body of the message
	JsonBody = 4096
)

// Errors
const (
	EMISSING = iota
)

var (
	// debug can be set to true to disable running of external commands
	debug bool

	// verbose can be set to true to enable more verbose logging
	verbose bool

	// timeout is the time we wait for each command run to complete before
	// canceling it.
	timeout time.Duration

	// config is a pointer to the global configuration object
	config *Configuration
)

// Alert represents an individual alert from Prometheus and included in the
// JSON blob POST'd via the Alertmanager.
type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt"`
	EndsAt       string            `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`

	// Timestamp is a string representing the time Alertmanager hit this
	// API.  Useful for logging.
	Timestamp string `json:"timestamp"`

	// Argv is not in the alert JSON and is available so the handler arguments
	// can be exposed to the template.
	Argv []string `json:"-"`

	// Json is not from the alert JSON but holds a JSON formatted string
	// of this alert.  It is not the same JSON as originally passed in.
	Json string `json:"-"`
}

// AlertManagerEvent represents the JSON struct that is POST'd to a web_hook
// receiver from Prometheus' Alertmanager.  There are other fields in the
// JSON blob that are not included here.
type AlertManagerEvent struct {
	Version     string
	Status      string
	Receiver    string
	ExternalURL string
	Alerts      []Alert
}

// Configuration is the Golang type that represents the YAML structure of
// the configuration file.
type Configuration struct {
	// Handlers is a hash of handler name to the definition of what will
	// be executed.
	Handlers map[string]struct {

		// Command is the go template string of the command to execute
		Command string

		// Status is the status of the alert, either "firing" or "resolved",
		// that will trigger the handler execution.  A "*" character selects
		// any alert status.
		Status string
	}
}

// Error handling
type EventError struct {
	code   int
	object string
}

func (e EventError) Error() string {
	switch e.code {
	case EMISSING:
		return fmt.Sprintf("Handler %s is not defined or missing from configration",
			e.object)
	}

	return "Undefined event error"
}

// replace is a helper function for templating to do simple substitution.
func replace(a, b, c string) string {
	return strings.Replace(a, b, c, -1)
}

// loadConfiguration reads YAML data from the specified file name and populates
// a Configuration object.
func loadConfiguration(file string) (*Configuration, error) {
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	body := make([]byte, JsonBody)
	size := 0
	size, err = fd.Read(body)
	if err != nil && err != io.EOF {
		return nil, err
	}

	cfg := new(Configuration)
	err = yaml.Unmarshal(body[:size], cfg)
	if err != nil {
		cfg = nil
	}
	return cfg, err
}

// formatHandler is a helper function to handle rendering the handler string
// templates.
func formatHandler(handler []string, command string, a Alert) (string, []string, error) {
	funcs := template.FuncMap{"replace": replace}
	// We ignore handler[0] as its the handle looked up to find command
	a.Argv = handler[1:]

	tmpl, err := template.New("command").Funcs(funcs).Parse(command)
	if err != nil {
		log.Printf("Error: Template parsing failed for \"%s\" with error: %s",
			command, err)
		return "", nil, err
	}
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, a)
	if err != nil {
		log.Printf("Error: Template execution failed for \"%s\" with error: %s",
			command, err)
		return "", nil, err
	}

	// Tokenize here to preserve quoted arguments
	fields, err := Tokenize(buf.String())
	if err != nil {
		return "", nil, err
	}
	return fields[0], fields[1:], nil
}

// executeHandler executes a handler give an executable and a slice of
// arguments.  STDOUT and STDERR are merged together and returnd in the
// bytes.Buffer.
func executeHandler(exe string, args []string) (*bytes.Buffer, error) {
	done := make(chan error, 1)
	var err error
	if debug {
		log.Printf("DEBUG: Not executing command \"%s\" with args \"%#v\"", exe, args)
		return nil, nil
	}

	out := new(bytes.Buffer)
	cmd := exec.Command(exe, args...)
	cmd.Stderr = out
	cmd.Stdout = out
	start := time.Now().Unix()
	if err = cmd.Start(); err != nil {
		return nil, err
	}

	// This must be a channel to work with select() to implement a timeout
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err = <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill() // Ignore error here
		err = fmt.Errorf("Command execution timed out and was killed.")
		out = nil
	}

	end := time.Now().Unix()
	if err != nil {
		log.Printf("Command \"%s\" Args \"%#v\" failed in %d seconds: %s",
			exe, args, end-start, err.Error())
	} else {
		log.Printf("Command \"%s\" Args \"%#v\" ran successfully in %d seconds",
			exe, args, end-start)
	}

	return out, err
}

// handleEvent does the initial work to handle events from the HTTP body.
func handleEvent(e *AlertManagerEvent) (*bytes.Buffer, error) {
	errors := 0
	retText := new(bytes.Buffer)
	for _, alert := range e.Alerts {
		log.Printf("Processing Alert: %s", alert.Labels["alertname"])
		var handler []string
		alert.Timestamp = time.Now().UTC().Format(time.RFC3339)

		buf, err := json.Marshal(alert)
		if err != nil {
			msg := fmt.Sprintf("Error marshalling JSON: %s", err.Error())
			log.Print(msg)
			retText.WriteString(msg + "\n")
			errors++
			continue
		}
		alert.Json = string(buf)
		if _, ok := alert.Annotations["handler"]; !ok {
			// We didn't find the "handler" annotation
			log.Printf("%s does not have handler annotation trying default",
				alert.Labels["alertname"])
			handler = []string{"default"}
		} else {
			handler = strings.Fields(alert.Annotations["handler"])
		}

		// Run our handler or the default if no handler is present.  Following
		// that run the "all" handler if present.
		for _, h := range [][]string{handler, []string{"all"}} {
			output, err := parseHandler(h, alert)
			if err != nil {
				if e, ok := err.(EventError); ok && e.code == EMISSING {
					if h[0] == "default" || h[0] == "all" {
						// Ignore missing handler errors for our special handlers
						// This means that a missing handler annotation is not
						// considered an error.
						continue
					}
				}
				log.Printf(err.Error())
				retText.WriteString(err.Error() + "\n")
				errors++
			}
			if output != nil && output.Len() > 0 {
				retText.Write(output.Bytes())
			}
		}
	}

	if errors > 0 {
		return retText, fmt.Errorf("Error(s) executing event(s)")
	}

	return retText, nil
}

// parseHandler parses and error checks the handler string before execution.
func parseHandler(handler []string, alert Alert) (*bytes.Buffer, error) {
	if len(handler) == 0 {
		return nil, fmt.Errorf("Empty handler annotation found in alert.")
	}
	command, ok := config.Handlers[handler[0]]
	if !ok {
		return nil, EventError{EMISSING, handler[0]}
	}
	if command.Status == "" {
		// Set default value for non-specified status
		command.Status = "firing"
	}
	if command.Status != "*" && command.Status != alert.Status {
		log.Printf("Ignoring alert.  Status (%s) which does not match filter (%s)",
			alert.Status, command.Status)
		return nil, nil
	}
	script, args, err := formatHandler(handler, command.Command, alert)
	if err != nil {
		return nil, fmt.Errorf("Could not parse handler arguments: %s", err.Error())
	}
	if script == "" {
		// Sanity
		return nil, fmt.Errorf("Script is empty, not running.")
	}

	return executeHandler(script, args)
}

// unmarshalBody is a helper function to load JSON from an HTTP body into
// an AlertManagerEvent structure.
func unmarshalBody(encoded []byte) (*AlertManagerEvent, error) {
	data := new(AlertManagerEvent)
	err := json.Unmarshal(encoded, &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// amWebHook decodes the HTTP request, finds Alertmanager JSON structure
// and dispatches the alerts.
func amWebHook(writer http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error
	var n int

	// Log the request
	w := NewStatusResponseWriter(writer)
	defer logRequest(w, r)

	// Filter requests for POST
	if r.Method != "POST" {
		http.Error(w, "Bad request method.", http.StatusBadRequest)
		return
	}

	buf := make([]byte, JsonBody)
	for err == nil {
		n, err = r.Body.Read(buf)
		if err != nil && err != io.EOF {
			log.Printf("Error reading from client: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if n > 0 {
			body = append(body, buf[:n]...)
		}
	}

	if verbose {
		log.Printf("Request Body: \"%s\"", string(body))
	}

	event, err := unmarshalBody(body)
	if err != nil {
		log.Printf("Error parsing request JSON: %s", err.Error())
		http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	output, err := handleEvent(event)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if output.Len() > 0 {
		blob := output.Bytes()
		w.Write(blob)
		if verbose {
			log.Printf("Response body: %s", string(blob))
		}
	}
}

// run starts the HTTP server
func run(bindAddress string) {
	http.HandleFunc("/", amWebHook)

	log.Printf("Starting server on %s", bindAddress)
	err := http.ListenAndServe(bindAddress, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	var bindAddress string
	var configFile string
	var err error

	flag.StringVar(&bindAddress, "bind", "0.0.0.0:4242",
		"IP:PORT to listen for HTTP requests.")
	flag.StringVar(&bindAddress, "b", "0.0.0.0:4242",
		"IP:PORT to listen for HTTP requests.")
	flag.StringVar(&configFile, "config", "./config.yaml",
		"Configuration file.")
	flag.StringVar(&configFile, "c", "./config.yaml",
		"Configuration file..")
	flag.BoolVar(&debug, "debug", false, "Activate debug mode.")
	flag.BoolVar(&debug, "d", false, "Activate debug mode.")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging.")
	flag.BoolVar(&verbose, "v", false, "Verbose logging.")
	flag.DurationVar(&timeout, "timeout", time.Second*30, "Command/Handler timeout.")
	flag.DurationVar(&timeout, "t", time.Second*30, "Command/Handler timeout.")

	flag.Parse()
	config, err = loadConfiguration(configFile)
	if err != nil {
		log.Fatalf("Configuration error, aborting: %s", err)
	}
	for k, v := range config.Handlers {
		log.Printf("Found handler %s => %s", k, v)
	}

	run(bindAddress)
}
