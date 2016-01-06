// Package xcf implements a simple reader for XCF files. This is the native
// format of the GIMP program.
//
// The current implementation only supports loading RGB and RGBA layers.
//
// The implementation uses information from
//    http://henning.makholm.net/xcftools/xcfspec-saved
// as a reference.
package xcf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"os"
)

// A Canvas contains all the image's layers. It is not itself an image, it just
// has a size and its top-left corner has the coordinates 0,0. The layers' bounds
// are relative to that. Layers are ordered top-most first and bottom-most last
// in the Layers slice.
type Canvas struct {
	Width, Height uint32
	Layers        []Layer
}

// GetLayerByName searches all layers for one with the given name and reutrns it.
// If no layer with that name exists, nil is returned.
func (c *Canvas) GetLayerByName(name string) *Layer {
	for i := range c.Layers {
		if c.Layers[i].Name == name {
			return &c.Layers[i]
		}
	}
	return nil
}

// A Layer is a rectangular pixel area. Its bounds are relative to the Canvas'
// origin, which is at 0,0. The bounds can lie outside the canvas.
// Each layer is itself an image.Image. The image data contains the unmodified
// pixels, the Visibility and Opacity are stored separately. You have to mask in
// the opacity if you want to draw the layer onto another image.
type Layer struct {
	image.Image
	Name    string
	Visible bool
	Opacity uint8
}

// LoadFromFile opens the given file and reads the XCF data using the Decode
// function. See that function for restrictions.
func LoadFromFile(path string) (Canvas, error) {
	file, err := os.Open(path)
	if err != nil {
		return Canvas{}, err
	}
	defer file.Close()

	return Decode(file)
}

// Decode reads a canvas from the given ReadSeeker. It currently supports RGB
// and RGBA pixel formats, file format version "file" and RLE encoded data only.
func Decode(r io.ReadSeeker) (canvas Canvas, finalErr error) {
	// read header
	var header fileHeader

	if err := binary.Read(r, endianness, &header); err != nil {
		finalErr = errors.New("read XCF: error reading header: " + err.Error())
		return
	}

	magic := string(header.MagicID[:])
	if magic != "gimp xcf " {
		finalErr = errors.New("read XCF: wrong magic ID: " + magic)
		return
	}

	version := string(header.Version[:])
	if version != "file" {
		finalErr = errors.New("read XCF: unsupported file version: " + version)
		return
	}

	if header.ColorFormat != rgb {
		finalErr = fmt.Errorf("read XCF: unsupported color format: %v", header.ColorFormat)
		return
	}

	// read properties
	for {
		var propHeader propertyHeader
		if err := binary.Read(r, endianness, &propHeader); err != nil {
			finalErr = errors.New("read XCF: error reading property header: " + err.Error())
			return
		}

		if propHeader.MagicID == propEnd {
			break
		} else if propHeader.MagicID == propColormap {
			// NOTE skipping the color map cannot rely on the property header
			// length since there are broken GIMP versions that write it wrong
			var colorCount uint32
			if err := binary.Read(r, endianness, &colorCount); err != nil {
				finalErr = errors.New("read XCF: unable to read number of colors in color map: " + err.Error())
				return
			}
			if _, err := io.CopyN(ioutil.Discard, r, int64(3*colorCount)); err != nil {
				finalErr = errors.New("read XCF: error skipping color map: " + err.Error())
				return
			}
		} else if propHeader.MagicID == propCompression {
			var compression [1]byte
			if _, err := r.Read(compression[:]); err != nil {
				finalErr = errors.New("read XCF: error reading compression: " + err.Error())
				return
			}
			if compression[0] != rleCompression {
				finalErr = fmt.Errorf("read XCF: unsupported compression: %v", compression[0])
				return
			}
		} else {
			// skip unknown properties
			if err := skipProperty(r, propHeader); err != nil {
				finalErr = errors.New("unable to skip property: " + err.Error())
			}
		}
	}

	// read layers
	var layerPointers []uint32 // top-most layer comes first
	var ptr uint32
	for {
		binary.Read(r, endianness, &ptr)
		if ptr == 0 {
			break
		}
		layerPointers = append(layerPointers, ptr)
	}

	var layers []Layer
	for _, ptr := range layerPointers {
		layer, err := loadLayer(r, ptr)
		if err != nil {
			finalErr = errors.New("read XCF: error reading layers: " + err.Error())
			return
		}
		layers = append(layers, layer)
	}

	return Canvas{
		Width:  header.Width,
		Height: header.Height,
		Layers: layers,
	}, nil
}

var endianness = binary.BigEndian

func skipProperty(r io.Reader, header propertyHeader) error {
	_, err := io.CopyN(ioutil.Discard, r, int64(header.Length))
	if err != nil {
		return errors.New("read XCF: unable to skip property: " + err.Error())
	}
	return nil
}

type fileHeader struct {
	MagicID           [9]byte
	Version           [4]byte
	VersionTerminator byte
	Width, Height     uint32
	ColorFormat       uint32
}

// color formats
const (
	// main image color formats
	rgb       = 0
	grayscale = 1
	indexed   = 2
	// layer color formats
	rgbNoAlpha       = 0
	rgbAlpha         = 1
	grayscaleNoAlpha = 2
	grayscaleAlpha   = 3
	indexedNoAlpha   = 4
	indexedAlpha     = 5
)

type propertyHeader struct {
	MagicID uint32
	Length  uint32
}

// property types
const (
	propEnd               = 0
	propColormap          = 1
	propActiveLayer       = 2
	propActiveChannel     = 3
	propSelection         = 4
	propFloatingSelection = 5
	propOpacity           = 6
	propMode              = 7
	propVisible           = 8
	propLinked            = 9
	propLockAlpha         = 10
	propApplyMask         = 11
	propEditMask          = 12
	propShowMask          = 13
	propShowMasked        = 14
	propOffsets           = 15
	propColor             = 16
	propCompression       = 17
	propGuides            = 18
	propResolution        = 19
	propTattoo            = 20
	propParasites         = 21
	propUnit              = 22
	propPaths             = 23
	propUserUnit          = 24
	propVectors           = 25
	propTextLayerFlags    = 26
	propSamplePoints      = 27
	propLockContent       = 28
	propGroupItem         = 29
	propItemPath          = 30
	propGroupItemFlags    = 31
)

// compression types
const (
	noCompression      = 0
	rleCompression     = 1
	zlibCompression    = 2
	fractalCompression = 3
)

type layerHeader struct {
	Width, Height uint32
	ColorFormat   uint32
}

func readString(r io.Reader) (string, error) {
	var length uint32
	if err := binary.Read(r, endianness, &length); err != nil {
		return "", errors.New("reading string length: " + err.Error())
	}
	// the empty string is stored simply as the number 0
	if length == 0 {
		return "", nil
	}

	// the length includes a 0 terminator that has to be read as well
	data := make([]byte, length)
	_, err := r.Read(data)
	if err != nil {
		return "", errors.New("reading string: " + err.Error())
	}

	// there is a 0 terminator at the end of the string, ignore that
	return string(data[:length-1]), nil
}

func loadLayer(r io.ReadSeeker, offset uint32) (layer Layer, err error) {
	var header layerHeader
	_, err = r.Seek(int64(offset), 0)
	if err != nil {
		return
	}

	if err = binary.Read(r, endianness, &header); err != nil {
		return
	}
	if !(header.ColorFormat == rgbAlpha || header.ColorFormat == rgbNoAlpha) {
		err = fmt.Errorf("unsupported layer color format, must be RGB: %v", header.ColorFormat)
		return
	}

	if layer.Name, err = readString(r); err != nil {
		return
	}

	// read properties
	layer.Visible = true
	var x, y int32
	for {
		var propHeader propertyHeader
		if err = binary.Read(r, endianness, &propHeader); err != nil {
			return
		}

		if propHeader.MagicID == propEnd {
			break
		} else if propHeader.MagicID == propOffsets {
			if err = binary.Read(r, endianness, &x); err != nil {
				return
			}
			if err = binary.Read(r, endianness, &y); err != nil {
				return
			}
		} else if propHeader.MagicID == propVisible {
			var visible uint32
			if err = binary.Read(r, endianness, &visible); err != nil {
				return
			}
			layer.Visible = visible != 0
		} else if propHeader.MagicID == propOpacity {
			var opacity uint32
			if err = binary.Read(r, endianness, &opacity); err != nil {
				return
			}
			layer.Opacity = uint8(opacity)
		} else {
			if err = skipProperty(r, propHeader); err != nil {
				return
			}
		}
	}

	var pixelPointer uint32
	if err = binary.Read(r, endianness, &pixelPointer); err != nil {
		return
	}

	var maskPointer uint32
	if err = binary.Read(r, endianness, &maskPointer); err != nil {
		return
	}

	layer.Image, err = readImageData(r, header, int(x), int(y), pixelPointer)
	if err != nil {
		return
	}

	return
}

type hierarchyHeader struct {
	Width, Height     uint32
	BytesPerPixel     uint32
	FirstLevelPointer uint32
}

func readImageData(r io.ReadSeeker, layerHeader layerHeader, layerX, layerY int,
	offset uint32) (img image.Image, err error) {
	var header hierarchyHeader
	r.Seek(int64(offset), 0)
	if err = binary.Read(r, endianness, &header); err != nil {
		return
	}

	// read unused level pointers
	var ptr uint32
	for {
		if err = binary.Read(r, endianness, &ptr); err != nil {
			return
		}
		if ptr == 0 {
			break
		}
	}

	// read first level data
	r.Seek(int64(header.FirstLevelPointer), 0)
	var levelWidth, levelHeight uint32 // redundant, same as in layerHeader
	if err = binary.Read(r, endianness, &levelWidth); err != nil {
		return
	}
	if err = binary.Read(r, endianness, &levelHeight); err != nil {
		return
	}

	tileCountX := int(layerHeader.Width+63) / 64
	tileCountY := int(layerHeader.Height+63) / 64
	rightMostTileWidth := int(layerHeader.Width) % 64
	if rightMostTileWidth == 0 {
		rightMostTileWidth = 64
	}
	bottomMostTileHeight := int(layerHeader.Height) % 64
	if bottomMostTileHeight == 0 {
		bottomMostTileHeight = 64
	}
	tileCount := tileCountX * tileCountY

	tilePointers := make([]uint32, tileCount+1) // + 1 for 0 terminator
	if err = binary.Read(r, endianness, tilePointers); err != nil {
		return
	}
	tilePointers = tilePointers[:tileCount] // remove 0 terminator

	rgba := image.NewRGBA(image.Rect(
		layerX,
		layerY,
		layerX+int(layerHeader.Width),
		layerY+int(layerHeader.Height),
	))

	buffer := make([]byte, 64*64*header.BytesPerPixel)
	for tileY := 0; tileY < tileCountY; tileY++ {
		for tileX := 0; tileX < tileCountX; tileX++ {
			left, top := tileX*64, tileY*64
			w, h := 64, 64
			if tileX == tileCountX-1 {
				w = rightMostTileWidth
			}
			if tileY == tileCountY-1 {
				h = bottomMostTileHeight
			}
			destByteCount := w * h * int(header.BytesPerPixel)
			data := buffer[:destByteCount]
			if err = decodeRLE(r, data); err != nil {
				return
			}

			dataIndex := 0
			for y := top; y < top+h; y++ {
				line := rgba.Pix[rgba.Stride*y:]
				if layerHeader.ColorFormat == rgbAlpha {

					ofs := len(data) / 4
					for x := left; x < left+w; x++ {
						line[x*4] = uint8(data[dataIndex])
						line[x*4+1] = uint8(data[dataIndex+ofs])
						line[x*4+2] = uint8(data[dataIndex+2*ofs])
						line[x*4+3] = uint8(data[dataIndex+3*ofs])
						dataIndex++
					}

				} else if layerHeader.ColorFormat == rgbNoAlpha {

					ofs := len(data) / 3
					for x := left; x < left+w; x++ {
						line[x*4] = uint8(data[dataIndex])
						line[x*4+1] = uint8(data[dataIndex+ofs])
						line[x*4+2] = uint8(data[dataIndex+2*ofs])
						line[x*4+3] = 255
						dataIndex++
					}

				}
			}
		}
	}
	img = rgba

	return
}

func decodeRLE(encoded io.Reader, dest []byte) (err error) {
	var buffer [4]byte
	next := 0

	for next < len(dest) {
		if _, err = encoded.Read(buffer[:1]); err != nil {
			return
		}
		op := buffer[0]
		if op <= 126 {
			// short run of identical bytes
			if _, err = encoded.Read(buffer[1:2]); err != nil {
				return
			}
			count := int(op) + 1
			value := buffer[1]
			for i := 0; i < count; i++ {
				dest[next] = value
				next++
			}
		} else if op == 127 {
			// long run of identical bytes
			if _, err = encoded.Read(buffer[1:4]); err != nil {
				return
			}
			count := int(buffer[1])*256 + int(buffer[2])
			value := buffer[3]
			for i := 0; i < count; i++ {
				dest[next] = value
				next++
			}
		} else if op == 128 {
			// long run of different bytes, copied verbatim from the stream
			if _, err = encoded.Read(buffer[1:3]); err != nil {
				return
			}
			count := int(buffer[1])*256 + int(buffer[2])
			if _, err = encoded.Read(dest[next : next+count]); err != nil {
				return
			}
			next += count
		} else {
			// short run of different bytes, copied verbatim from the stream
			count := 256 - int(op)
			if _, err = encoded.Read(dest[next : next+count]); err != nil {
				return
			}
			next += count
		}
	}
	return nil
}
