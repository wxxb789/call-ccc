package main

// upstreamRevision is the AgentsView revision this binary's inherited source side was carried at.
//
// scripts/upstream-parity.py fails if this constant stops matching the pin recorded in
// scripts/upstream-manifest.txt, so a pin update cannot leave the revision this binary reports
// behind. See docs/upstream.md for the pin, the declared deviations and the update procedure.
const upstreamRevision = "83195c8a6861b8c74a4b6cb87512817fed6f8584"
