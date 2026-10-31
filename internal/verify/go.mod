module github.com/dchupp/DataMagik-ZPL-To-Image/internal/verify

go 1.26.10

require (
	github.com/dchupp/DataMagik-ZPL-To-Image v0.0.0
	github.com/makiuchi-d/gozxing v0.1.1
)

require (
	github.com/dchupp/DataMagik-ZPL-To-Image/fonts/notocjk v0.0.0
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)

replace github.com/dchupp/DataMagik-ZPL-To-Image => ../..

replace github.com/dchupp/DataMagik-ZPL-To-Image/fonts/notocjk => ../../fonts/notocjk
