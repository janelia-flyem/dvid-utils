/*
   This conversion program adds a DVID data set from a directory of Raveler tiles.
   Raveler tile directories are arranged in the following fashion:

   /tiles
     /1024
       /0
         /y (0..MaxY/TileSizeY)
           /x (0..MaxX/TileSizeX)
             /g
               <z>.png
               /1000
                 <z>.png
               /2000
                 <z>.png
             /s
                <z>.png
               /1000
                 <z>.png
*/
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/janelia-flyem/dvid/dvid"
)

var (
	// Load only superpixels if true
	loadSuperpixels = flag.Bool("superpixels", false, "")

	// Load only grayscale if true
	loadGrayscale = flag.Bool("grayscale", false, "")

	// Display usage if true.
	showHelp = flag.Bool("help", false, "")
)

const tileSize = 1024

const helpMessage = `
%s adds a DVID data set from a set of images.

Usage: %s [options] <uuid> <dataset name> <tiles dir> <offset x,y,z> <size x,y,z>

  -s, -superpixels  (flag)   Load only superpixel tiles
  -g, -grayscale    (flag)   Load only grayscale tiles
  -h, -help         (flag)   Show help message
`

var usage = func() {
	programName := filepath.Base(os.Args[0])
	fmt.Printf(helpMessage, programName, programName)
}

func runCommand(args ...string) (err error) {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if len(out) > 0 {
		fmt.Println(string(out))
	}
	return
}

func runAsyncCommand(args ...string) (err error) {
	cmd := exec.Command(args[0], args[1:]...)
	err = cmd.Start()
	return
}

// TileFilename returns the path to a given tile relative to tiles directory.
func tileFilename(row, col, slice int32, superpixel bool) (filename string) {
	var typeDir string
	if superpixel {
		typeDir = "s"
	} else {
		typeDir = "g"
	}
	if slice >= 1000 {
		sliceDir := (slice / 1000) * 1000
		filename = fmt.Sprintf("%d/0/%d/%d/%s/%d/%d.png", tileSize,
			row, col, typeDir, sliceDir, slice)
	} else {
		filename = fmt.Sprintf("%d/0/%d/%d/%s/%03d.png", tileSize,
			row, col, typeDir, slice)
	}
	return
}

func main() {
	flag.BoolVar(showHelp, "h", false, "Show help message")
	flag.Usage = usage
	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	// Get required arguments
	args := flag.Args()
	if len(args) < 5 {
		usage()
		os.Exit(2)
	}
	uuidStr := args[0]
	datasetName := args[1]
	tilesDir := args[2]
	offsetStr := args[3]
	sizeStr := args[4]

	fmt.Println("Tiles directory:", tilesDir)
	offset, err := dvid.PointStr(offsetStr).VoxelCoord()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Offset: %s\n", offset)
	size, err := dvid.PointStr(sizeStr).Point3d()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Size: %s\n", size)

	// Add tiles within the bounds.
	endVoxel := offset.AddSize(size)
	startTileX := offset[0] / tileSize
	startTileY := offset[1] / tileSize
	endTileX := endVoxel[0] / tileSize
	endTileY := endVoxel[1] / tileSize

	for z := offset[2]; z <= endVoxel[2]; z++ {
		numTiles := 0
		for tileY := startTileY; tileY <= endTileY; tileY++ {
			for tileX := startTileX; tileX <= endTileX; tileX++ {
				offsetStr := fmt.Sprintf("%d,%d,%d", tileX*tileSize, tileY*tileSize, z)
				if *loadSuperpixels {
					filename := filepath.Join(tilesDir, tileFilename(tileY, tileX, z, true))
					err = runCommand("dvid", datasetName, "server-add", uuidStr, offsetStr, filename)
					if err != nil {
						log.Fatalf("Error in command: %s\n", err.Error())
					}
				}
				if *loadGrayscale {
					filename := filepath.Join(tilesDir, tileFilename(tileY, tileX, z, false))
					err = runCommand("dvid", datasetName, "server-add", uuidStr, offsetStr, filename)
					if err != nil {
						log.Fatalf("Error in command: %s\n", err.Error())
					}
				}
				numTiles++
			}
		}
		fmt.Printf("Added %d %s tiles from z = %d...\n", numTiles, datasetName, z)
	}
}
