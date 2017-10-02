package main

import (
    "net/http"
    "os"
    "fmt"
    "os/exec"
    "bytes"
    "io/ioutil"
    "path/filepath"
    "flag"
    "strings"
)

// Import module to use perl compatible regular expressions
import "github.com/glenn-brown/golang-pkg-pcre/src/pkg/pcre"

// A type which stores a path and file info, used to pass this data
// accross a channel to each of the worker processes
type workerInfo struct{
    path string
    info os.FileInfo
}

// Map of file extensions that should be skipped
var rejectMap = map[string]bool {
    ".c": true,
    ".h": true,
    ".dox": true,
    ".html": true,
    ".rst": true,
    ".chain": true,
    ".cpp": true,
    ".cc": true,
    ".xml": true,
    ".hpp": true,
    ".fits": true,
    ".js": true,
    ".png": true,
    ".css": true,
}


// Channel which to send info to each of the worker processes
var channel = make(chan workerInfo, 100)

func processPath(data <-chan workerInfo, worker int, newPython []byte, listFlag bool) {
    /*
        This function recieves a file path and file info from a channel
        and does the actuall processing and shebang replacement. This is
        intended to be used as a worker go routine.
    */
    // Compile the regular expression to find the python shebang line
    // excludes anython with /usr/bin/env, as these possible should
    // remain (for things likes scons itself)
    REXP, _ := pcre.Compile("^#[!]((?!\\s?[/]usr[/]bin[/]env).*python)", 0)
    // loop forever extracting objects from the channel
    for entry := range data {
        // Get the path and file info to work on
        info := entry.info
        path := entry.path
        // Verify that this is a regular type of file
        if os.ModeType & info.Mode() != 0 || info.IsDir() == true {
            continue
        }
        // check if the path extention is in the ignore list
        if _, ok := rejectMap[filepath.Ext(path)]; ok {
            continue
        }
        // Open the file for reading, fetch the first 512 bytes and
        // verify that this is a text type file.
        file, fileErr := os.Open(path)
        if fileErr != nil {
            continue
        }
        buffer := make([]byte, 512)
        n, buffErr := file.Read(buffer)
        // Close the file handle used to determine if the object is text
        file.Close()
        if buffErr != nil {
            continue
        }
        contentType := http.DetectContentType(buffer[:n])
        if contentType == "text/plain; charset=utf-8" {
            // If the file is a standard text based file
            // read in the file as bytes and replace any isinstance
            // of the old shebang line with the new case.
            fileBytes, fileBytesErr := ioutil.ReadFile(path)
            if fileBytesErr != nil {
                continue
            }
            if !listFlag {
                newFile := REXP.ReplaceAll(fileBytes, newPython, 0)
                // only write a new file if a replacement has
                // happened. It would be better if ReplaceAll
                // reported this so compareing would not be
                // needed, however it is what it is
                if bytes.Compare(fileBytes, newFile) != 0 {
                    ioutil.WriteFile(path, newFile, info.Mode())
                }
            }
            if listFlag {
                matcher := REXP.Matcher(fileBytes, 0)
                if matcher.Matches() {
                    //fmt.Println("File: ", path, "Shebang: ",
                     //           matcher.GroupString(1), worker)
                }
            }
        }
    }
}

func dispatch(path string, info os.FileInfo, err error) error {
    /*
        This function serves as a dispatcher to each of the worker go routines.
        It takes input from the file system walker, and puts it in the buffered
        channel for each of the routines to pick up.
    */
    channel <- workerInfo{path, info}
    return nil
}

func main() {
    /*
        A eups stack must be setup prior to running this script.
        i.e. source $EUPS_PATH/loadLSST.<ext>
    */
    // Get the command line switch to see if we should just list shebangs found
    // in files
    listFlag := flag.Bool("list", false, "Only display a list of shebangs found")
    flag.Usage = func() {
        helpString := `Shebangtron: Used to rewrite python path in the lsst stack on MacOS.
             Must be run after sourceing loadLSST.<shell>`
        fmt.Println(helpString)
        flag.PrintDefaults()
    }

    flag.Parse()
    // Get the root directory under which the stack is installed
    rootPath := os.Getenv("EUPS_PATH")
    flavorOutput := &bytes.Buffer{}
    flavorCmd := exec.Command("eups", "flavor")
    flavorCmd.Stdout = flavorOutput
    flavErr := flavorCmd.Run()
    if flavErr != nil {
        fmt.Println(flavErr)
        os.Exit(1)
    }
    flavor := string(flavorOutput.Bytes())
    flavor = strings.Replace(flavor, "\n", "", 1)
    // Determine from the system the proper path path for python is
    cmdOutput := &bytes.Buffer{}
    cmd := exec.Command("which", "python")
    cmd.Stdout = cmdOutput
    err := cmd.Run()
    newPython := cmdOutput.Bytes()
    newPython = bytes.Trim(newPython, "\n")
    // Prepend the #! to the python path
    shbang := []byte("#!")
    newPython = append(shbang[:], newPython[:]...)
    if err != nil {
        fmt.Println(err)
        os.Exit(1)
    }

    // Start the worker processes
    numWorkers := 10
    for i := 0; i < numWorkers; i++ {
        go processPath(channel, i, newPython, *listFlag)
    }

    // Walk the eups path and send the paths to the dispatcher
    totalPath := rootPath + "/"+ flavor
    err = filepath.Walk(totalPath, dispatch)
    if err != nil {
        fmt.Println(err)
        os.Exit(1)
    }
    fmt.Println("Shebang successfully updated")
    close(channel)
}
