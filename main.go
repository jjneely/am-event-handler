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

var (
	// debug dictates if we log additional debugging data
	debug bool

	// config is a pointer to the global configuration object
	config *Configuration
)

// Alert represents an individual alert from Prometheus and included in the
// JSON blob POST'd via the Alertmanager.
type Alert struct {
	Status       string
	Labels       map[string]string
	Annotations  map[string]string
	StartsAt     string
	GeneratorURL string

	// Not in JSON, explicitly so we can expose handler arguments to the
	// template.
	Argv []string `json:"-"`
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

type Configuration struct {
	Handlers map[string]string
}

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

func formatHandler(handler []string, command string, a Alert) (string, []string, error) {
	// We ignore handler[0] as its the handle looked up to find command
	a.Argv = handler[1:]

	tmpl, err := template.New("command").Parse(command)
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

func executeHandler(exe string, args []string) (*bytes.Buffer, error) {
	if debug {
		log.Printf("DEBUG: Not executing command \"%s\" with args \"%#v\"", exe, args)
		return nil, nil
	}

	cmd := exec.Command(exe, args...)
	out := new(bytes.Buffer)
	cmd.Stderr = out
	cmd.Stdout = out

	start := time.Now().Unix()
	err := cmd.Run()
	end := time.Now().Unix()
	if err != nil {
		log.Printf("Command \"%s\" Args \"%#v\" failed in %d seconds: %s",
			exe, args, end-start, err.Error())
		log.Printf("Error: %s", err)
		if out.Len() > 0 {
			log.Printf("%s", out.String())
		}
	} else {
		log.Printf("Command \"%s\" Args \"%#v\" ran successfully in %d seconds",
			exe, args, end-start)
	}

	return out, err
}

func handleEvent(e *AlertManagerEvent) error {
	if e.Status != "firing" {
		log.Printf("Ignoring %d non-firing alerts", len(e.Alerts))
		return nil
	}

	errors := 0
	retText := new(bytes.Buffer)
	for _, alert := range e.Alerts {
		log.Printf("Processing Alert: %s", alert.Labels["alertname"])
		if _, ok := alert.Annotations["handler"]; !ok {
			// We didn't find the "handler" annotation
			log.Printf("Alert does not have handler annotation")
			retText.WriteString("Alert does not have handler annotation.\n")
			errors++
			continue
		}
		handler := strings.Fields(alert.Annotations["handler"])
		if len(handler) == 0 {
			log.Printf("Empty handler annotation found in alert")
			retText.WriteString("Empty handler annotation found in alert.\n")
			errors++
			continue
		}
		command, ok := config.Handlers[handler[0]]
		if !ok {
			log.Printf("Error: Handler %s not found", handler[0])
			retText.WriteString(fmt.Sprintf("Error: Handler %s not found.\n",
				handler[0]))
			errors++
			continue
		}
		script, args, err := formatHandler(handler, command, alert)
		if err != nil {
			msg := fmt.Sprintf("Could not parse handler arguments: %s", err.Error())
			log.Printf("%s", msg)
			retText.WriteString(msg + ".\n")
			errors++
			continue
		}
		if script == "" {
			// Sanity
			log.Printf("Script is empty, not running")
			retText.WriteString("Script is empty, not running.\n")
			errors++
			continue
		}

		output, err := executeHandler(script, args)
		if err != nil {
			// we've already logged this error in execution
			s := fmt.Sprintf("Error running command \"%s\" with args %#v: %s\n",
				script, args, err.Error())
			retText.WriteString(s)
		}
		if output != nil && output.Len() > 0 {
			retText.WriteString("Command Output:\n")
			retText.Write(output.Bytes())
			retText.WriteString("\nEnd Command Output\n")
		}
	}

	if errors > 0 {
		return fmt.Errorf("%s", retText.String())
	}

	return nil
}

func unmarshalBody(encoded []byte) (*AlertManagerEvent, error) {
	data := new(AlertManagerEvent)
	err := json.Unmarshal(encoded, &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func amWebHook(writer http.ResponseWriter, r *http.Request) {
	// Log the request
	w := NewStatusResponseWriter(writer)
	defer logRequest(w, r)

	// Filter requests for POST
	if r.Method != "POST" {
		http.Error(w, "Bad request method.", http.StatusBadRequest)
		return
	}

	body := make([]byte, JsonBody)
	n, err := r.Body.Read(body)
	if err != nil && err != io.EOF {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body = body[:n]

	if len(body) == JsonBody {
		http.Error(w, "Message body larger than 4KiB.", http.StatusBadRequest)
		return
	}

	if debug {
		log.Printf("Request Body: \"%s\"", string(body))
	}

	event, err := unmarshalBody(body)
	if err != nil {
		http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	err = handleEvent(event)
	if err != nil {
		http.Error(w, "Error(s) executing event(s):\n"+err.Error(),
			http.StatusBadRequest)
	}
}

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
