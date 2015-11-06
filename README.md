# xcf
Package xcf is a pure Go implementation of a basic XCF (standard format used by GIMP) image file reader.
It reads layers with names, visibility and opacity properties and the RGBA image data.
It currently supports RGB and RGBA pixel formats.

# Installation
You can go get the xcf package by typing the following command into your command line:
`go get github.com/gonutz/xcf`

# Example

```Go
package main

import (
	"fmt"
	"github.com/gonutz/xcf"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("please provide an XCF as the first parameter")
		return
	}

	canvas, err := xcf.LoadFromFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	for _, layer := range canvas.Layers {
		fmt.Println(layer.Name, layer.Bounds())
	}
}
```

See the [documentation](https://godoc.org/github.com/gonutz/xcf) for details.
