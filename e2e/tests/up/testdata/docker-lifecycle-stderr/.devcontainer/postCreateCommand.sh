#!/bin/sh
# Regression fixture for the dropped-last-line bug (see the e2e spec
# "forwards the final lifecycle hook log line before the agent exits").
#
# Emit a burst of stderr lines ending with a marker, then fail. The agent
# forwards lifecycle hook output over the tunnel asynchronously; the burst
# keeps the sender busy so the marker is still queued when the hook exits
# non-zero. Without the flush-on-shutdown fix the agent tears down before the
# marker is sent and it is dropped (the observed drain window is only ~11
# lines, so the 300-line burst leaves a wide margin).
#
# The marker lives in this script rather than the inline postCreateCommand so
# it cannot leak into devpod's "failed to run <command>" error message and
# produce a false positive.
i=0
while [ "$i" -lt 300 ]; do
    echo "filler line $i" >&2
    i=$((i + 1))
done
echo DEVPOD_LIFECYCLE_FLUSH_MARKER >&2
exit 1
