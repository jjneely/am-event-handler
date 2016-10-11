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

Contributing
------------

Clone this repository and create a GitHub PR.

Authors and Copyright
---------------------

Copyright 2015 42 Lines, Inc.  Original author: Jack Neely <jjneely@42lines.net>
