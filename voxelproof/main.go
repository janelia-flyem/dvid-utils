/*
   This conversion program creates a DVID database from a VoxelProof data directory.
   The VoxelProof data directory is assumed to have the following structure:

   /medulla_testing_runlash4
       /comps
           /z
               /0 to 619
                   [0-6]-[0-6].png
       /z
           /0 to 619
               [0-6]-[0-6].jpg

   The "comps" directory contains label images and might have multiple tile sizes
   (e.g., 100x200 when using 100x100 tiles) if more than 8 bits are needed.
*/
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/janelia-flyem/dvid/dvid"
)

var (
	// Display usage if true.
	showHelp = flag.Bool("help", false, "")
)

const helpMessage = `
%s creates a DVID database from a VoxelProof data directory.

Usage: %s [options] <VoxelProof path> <z range> <tile size> <output db path>

  -h, -help       (flag)    Show help message

<VoxelProof path>       Path to VoxelProof data directory.
<z range>               Range of z slices in "zmin,zmax" format, e.g., "0,619" for z = 0 to 619.
<tile size>             Size of tile in "width,height" format, e.g., "100,100" for 100x100.
<output db path>        Path to output DVID datastore directory to create.
`

var usage = func() {
	programName := filepath.Base(os.Args[0])
	fmt.Printf(helpMessage, programName, programName)
}

func initDatastore(outputDir string) (uuidStr string) {
	cmd := exec.Command("dvid", "-datastore="+outputDir, "init")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	// Parse the init command output and get the root version UUID.
	reader := bufio.NewReader(stdout)
	for {
		var line []byte
		line, _, err = reader.ReadLine()
		if err != nil {
			log.Fatalln("Did not detect root version UUID in output of dvid init!")
		}
		var n int
		n, err = fmt.Sscanf(string(line), "Root node UUID: %s", &uuidStr)
		if err == nil && n == 1 {
			return
		}
	}
	return
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
	if len(args) < 3 {
		usage()
		os.Exit(2)
	}
	voxelProofDir := args[0]
	zrangeStr := args[1]
	tileSizeStr := args[2]
	outputDir := args[3]

	fmt.Println("VoxelProof data directory:", voxelProofDir)
	zrange, err := dvid.PointStr(zrangeStr).Point2d()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Z range %d -> %d\n", zrange[0], zrange[1])
	tileSize, err := dvid.PointStr(tileSizeStr).Point2d()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Tile size: %d x %d pixels\n", tileSize[0], tileSize[1])
	fmt.Println("Output DVID database:", outputDir)

	// Initialize the database
	uuidStr := initDatastore(outputDir)
	fmt.Printf("Initialized datastore with root version %s.\n", uuidStr)

	// Startup a server for this new datastore.  It will exit when this program exits.
	cmd := exec.Command("dvid", "-datastore="+outputDir, "serve")
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Making sure DVID server has started...")
	time.Sleep(3 * time.Second)

	// Shutdown the server no matter how we exit
	defer runCommand("dvid", "shutdown")

	// Capture ctrl+c and handle graceful shutdown (flushing of cache, etc.)
	ctrl_c := make(chan os.Signal, 1)
	go func() {
		for sig := range ctrl_c {
			log.Printf("Captured %v.  Shutting down...\n", sig)
			runCommand("dvid", "shutdown")
			os.Exit(0)
		}
	}()
	signal.Notify(ctrl_c, os.Interrupt)

	// Create dataset for grayscale
	runCommand("dvid", "dataset", "grayscale", "grayscale8")

	// Create dataset for labels
	runCommand("dvid", "dataset", "labels", "labels32")

	// Add grayscale
	vol := dvid.VoxelCoord{700, 700, 620}
	tileXMax := vol[0] / tileSize[0]
	tileYMax := vol[1] / tileSize[1]
	fmt.Printf("X Tiles from [0,%d), Y Tiles from [0,%d)\n", tileXMax, tileYMax)

	numTiles := tileXMax * tileYMax
	for z := zrange[0]; z <= zrange[1]; z++ {
		for y := int32(0); y < tileYMax; y++ {
			for x := int32(0); x < tileXMax; x++ {
				fname := fmt.Sprintf("%d-%d.png", y, x)
				tilepath := filepath.Join(voxelProofDir, "z", strconv.Itoa(int(z)), fname)
				ox := x * tileSize[0]
				oy := y * tileSize[1]
				oz := z - zrange[0]
				offsetStr := fmt.Sprintf("%d,%d,%d", ox, oy, oz)
				err = runCommand("dvid", "grayscale", "server-add",
					uuidStr, offsetStr, tilepath)
				if err != nil {
					log.Fatalf("Error in command: %s\n", err.Error())
				}
			}
		}
		fmt.Printf("Added %d grayscale tiles from z = %d...\n", numTiles, z)
	}

	// Add labels
	for z := zrange[0]; z <= zrange[1]; z++ {
		for y := int32(0); y < tileYMax; y++ {
			for x := int32(0); x < tileXMax; x++ {
				fname := fmt.Sprintf("%d-%d.png", y, x)
				tilepath := filepath.Join(voxelProofDir, "comps", "z",
					strconv.Itoa(int(z)), fname)
				ox := x * tileSize[0]
				oy := y * tileSize[1]
				oz := z - zrange[0]
				offsetStr := fmt.Sprintf("%d,%d,%d", ox, oy, oz)
				err = runCommand("dvid", "labels", "server-add",
					uuidStr, offsetStr, tilepath)
				if err != nil {
					log.Fatalf("Error in command: %s\n", err.Error())
				}
			}
		}
		fmt.Printf("Added %d label tiles from z = %d...\n", numTiles, z)
	}
}
