# Printer comparison fixtures

Each file isolates one behaviour that Zebra's documentation leaves ambiguous
or where Labelary and the documentation disagree. `internal/verify`'s
`zebra` harness renders each one on a real printer (stored with `^IS`, read
back with `^HG`, never printed) and compares it with this library:

    ZPL_PRINTER=10.0.20.200:9100 go test -tags zebra -run Zebra -v ./internal/verify

Fixtures are single formats sized for a 4-inch, 300 dpi printhead; the
harness reads the printer's density with `~HI` and renders to match.
