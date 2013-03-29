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

Usage: %s [options] <VoxelProof path> <z range> <tile size> <config json path> <output db path>

  -h, -help       (flag)    Show help message

<VoxelProof path>       Path to VoxelProof data directory.
<z range>               Range of z slices in "zmin,zmax" format, e.g., "0,619" for z = 0 to 619.
<tile size>             Size of tile in "width,height" format, e.g., "100,100" for 100x100.
<config json path>      JSON file used to configure DVID volume via "dvid init".
<output db path>        Path to output DVID datastore directory to create.
`

var usage = func() {
    programName := filepath.Base(os.Args[0])
    fmt.Printf(helpMessage, programName, programName)
}

func initDatastore(configFilename, outputDir string) (uuidStr string) {
    cmd := exec.Command("dvid", "init", "config="+configFilename, "dir="+outputDir)
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
    if len(args) < 4 {
        usage()
        os.Exit(2)
    }
    voxelProofDir := args[0]
    zrangeStr := args[1]
    tileSizeStr := args[2]
    configFilename := args[3]
    outputDir := args[4]

    fmt.Println("VoxelProof data direcory:", voxelProofDir)
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
    fmt.Println("Configuration JSON file:", configFilename)
    fmt.Println("Output DVID database:", outputDir)

    // Initialize the database
    uuidStr := initDatastore(configFilename, outputDir)
    fmt.Printf("Initialized datastore with root version %s.\n", uuidStr)

    // Startup a server for this new datastore.  It will exit when this program exits.
    cmd := exec.Command("dvid", "serve", "dir="+outputDir)
    if err := cmd.Start(); err != nil {
        log.Fatal(err)
    }
    fmt.Println("Making sure DVID server has started...")
    time.Sleep(3 * time.Second)

    // Create dataset for grayscale
    cmd = exec.Command("dvid", "dataset", "grayscale", "grayscale8")
    out, err := cmd.Output()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(out))

    // Create dataset for labels
    cmd = exec.Command("dvid", "dataset", "labels", "labels32")
    out, err = cmd.Output()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(out))

    // Read in the DVID volume configuration.
    vol, err := dvid.ReadVolumeJSON(configFilename)
    if err != nil {
        log.Fatal(err)
    }

    // Add grayscale
    tileXMax := vol.VolumeMax[0] / tileSize[0]
    tileYMax := vol.VolumeMax[1] / tileSize[1]
    fmt.Printf("X Tiles from [0,%d), Y Tiles from [0,%d)\n", tileXMax, tileYMax)

    for z := zrange[0]; z <= zrange[1]; z++ {
        for y := int32(0); y < tileYMax; y++ {
            for x := int32(0); x < tileXMax; x++ {
                fname := fmt.Sprintf("%d-%d.jpg", y, x)
                tilepath := filepath.Join(voxelProofDir, "z", strconv.Itoa(int(z)), fname)
                ox := x * tileSize[0]
                oy := y * tileSize[1]
                oz := z - zrange[0]
                offsetStr := fmt.Sprintf("%d,%d,%d", ox, oy, oz)
                cmd = exec.Command("dvid", "grayscale", "server-add", uuidStr, offsetStr, tilepath)
                out, err = cmd.Output()
                fmt.Println(string(out))
                if err != nil {
                    log.Fatal(err)
                }
            }
        }
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
                cmd = exec.Command("dvid", "labels", "server-add", uuidStr, offsetStr, tilepath)
                out, err = cmd.Output()
                fmt.Println(string(out))
                if err != nil {
                    log.Fatal(err)
                }
            }
        }
    }

    // Shutdown the server
    cmd = exec.Command("dvid", "shutdown")
    out, err = cmd.Output()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(out))
}
