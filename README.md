Alertmanager Event Handler
==========================

Use Prometheus' Alertmanager to handle arbitrary events.  Inspired by the
event handlers in Nagios.

This works as a generic web hook in Alertmanager.  Alerts routed to this
event handler must have an annotation `handler` defined who's value is
the event handler that should be executed.  Optionally, the annotation
`handler` value may include arbitrary arguments passed to the event handler.

    <handler> [arg1, [arg2 ...]]

The configuration file for `am-event-handler` contains a hash of known
handlers which maps to a Go templated string.  This string builds the
executable and arguments that will run as the user running `am-event-handler`.
This allows the passing of arbitrary data with reasonable security restrictions
in place.  Example:

    ALERT PrometheusInstanceDown
      IF up{job="prometheus", environment="prod"} == 0
      FOR 5m
      LABELS {
        severity="event"
      }
      ANNOTATIONS {
        summary="{{$labels.instance}} Prometheus Instance Down",
        description="The Prometheus Instance {{$labels.instance}} is down",
        runbook="https://wiki/prometheus-instance-down",
        handler="restart-prom {{$labels.instance}}"
      }

This alert would be routed to event handler receiver in the Alertmanager
by the `severity` label.  The receiver configuration might look like the
below:

    # A list of notification receivers.
    receivers:
      - name: foobar
        webhook_configs:
          - url: http://host:port/

Where the `host:port` is where `am-event-handler` is running.

The `am-event-handler` requires a configuration file that will map the
handlers into an executable.  These are not run through a shell and shell
redirection will not work, but you can use this to execute a shell.  The
following is an example configuration:

    handlers:
      restart-prom: "remctl {{ index .Argv 0 }} prom-restart"

Note that the supplied arguments are stored in the `Argv` slice of strings.

Meta Handlers
-------------

Handlers are defined in the configuration file and can be any arbitrary,
non-whitespace containing string.  The handler selected matches the first
word of the `handler` annotation exactly.  There is no fancy logic there.

However, there are two meta handlers that can be defined in the configuration
that affect what will be executed.

* `default`: A handler of this name will be executed when no handler
  annotation is present, or the requested handler cannot be found.
* `all`: This handler is run for all alerts whether they have a handler
  annotation or not.  It will be run in addition to (and after) any
  matching handler the alert requests.

Templating
----------

In the configuration file the command to be executed can be templated as
in the example above.  In addition to Go's built in [text template][1]
functionality the following are made available.

Variables:

* `.Status`: `string` The status of the alert.  This should be either "firing"
  or "resolved".
* `.Labels`: `map[string]string`  A map of the labels applied to this alert.
* `.Annotations`: `map[string]string`  A map of the annotations applied to
  this alert.
* `.StartsAt`: `string` The RFC 3339 date the alert began as from the
  Alertmanager.
* `.EndsAt`: `string` The RFC 3339 date the alert ended as from the
  Alertmanager.  May be "0001-01-01T00:00:00Z" when the alert is in progress.
* `.GeneratorURL`: `string` The URL to the originating Prometheus server and
  graph.
* `.Argv`: `[]string` A slice of strings holding the arguments present in
  the `handler` annotation.  The handler annotation is whitespace delimited
  and this slice begins at the first argument and does not include the name
  of the specified handler.
* `.Json`: `string` A JSON representation of this alert.  This is a filtered
  representation of the alert and not the complete JSON as provided by the
  Alertmanager.

Contributing
------------

Clone this repository and create a GitHub PR.

Authors and Copyright
---------------------

Copyright 2015 42 Lines, Inc.  Original author: Jack Neely <jjneely@42lines.net>

[1]: https://golang.org/pkg/text/template/
