package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

func workerCore(depMap *sync.Map, r *regexp.Regexp, product string) {
	// get the version associated with the product key
	version, _ := depMap.Load(product)
	depCommand := exec.Command("eups", "list", "-D", product, string(version.([]byte)))
	out, _ := depCommand.Output()
	// these are all the depandancies for the product supplied by the channel
	tempDepList := bytes.Split(out, []byte("\n"))
	// loop over these dependencies adding them to the depMap if they are not already
	// present
	for _, val := range tempDepList {
		if len(val) == 0 {
			continue
		}
		matches := r.FindAllSubmatch(val, -1)
		// the last entry in the newline split will be zero length and should be skipped
		if len(matches[0]) == 0 {
			continue
		}
		_, ok := depMap.LoadOrStore(string(matches[0][1]), matches[0][2])
		// this product was stored, and so the dependancies for it should be
		// fetched
		if ok == false {
			workerCore(depMap, r, string(matches[0][1]))
		}
	}
}

func depWorker(depMap *sync.Map, channel chan string, wg *sync.WaitGroup, worker int) {
	/*
	   This function takes in a product fromt the communication channel and processes it
	   looking for dependancies not already in the depencancy map
	*/
	defer wg.Done()
	r, _ := regexp.Compile("([^|][\\S]+)\\s+(\\S+)")
	for product := range channel {
		workerCore(depMap, r, product)
	}
}

func updateDep(depMap *sync.Map) {
	// number of workers to use in looking up dependencies
	numWorkers := 4
	channel := make(chan string, numWorkers)
	// waitgroup to be sure that all the worker processes have finished their work
	var wg sync.WaitGroup
	// start the workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go depWorker(depMap, channel, &wg, i)
	}
	// loop through the keys in the depMap and dispatch them to a worker
	depMap.Range(func(key, value interface{}) bool {
		channel <- key.(string)
		return true
	})
	// Close the channnel
	close(channel)
	// Wait for all the workers to finish
	wg.Wait()
}

func undeclare(depMap *sync.Map) {
	/*
	   Function to undeclare all the unneeded products from eups
	*/
	// loop over each of the products in the dependency map
	depMap.Range(func(key, value interface{}) bool {
		// get all the versions from the particular product
		versionsCommand := exec.Command("eups", "list", key.(string))
		out, _ := versionsCommand.Output()
		versionsList := bytes.Split(out, []byte("\n"))
		r, _ := regexp.Compile("(\\S+)\\s*")
		// loop over all the versions, undeclare all versions except the one
		// that is in the dependency map, or products that start with tag:
		for _, val := range versionsList {
			// handle a zero length value (last element in the newline split)
			if len(val) == 0 {
				continue
			}
			matches := r.FindAllSubmatch(val, -1)
			if bytes.Compare(matches[0][1], value.([]byte)) != 0 &&
				bytes.Contains(matches[0][1], []byte("tag:")) == false {
				fmt.Println("Would undeclare: ", key.(string), string(matches[0][1]))
				undeclareCmd := exec.Command("eups", "undeclare", key.(string), string(matches[0][1]))
				_, err := undeclareCmd.Output()
				if err != nil {
					fmt.Println("Problem undeclaring ", key.(string), string(matches[0][1]))
				}
			}
		}
		return true
	})
}

func removeDirs(depMap *sync.Map) {
	/*
	   This function removes the directories containing the binaries (or python code)
	   from the file system corresponding to versions of products that have been
	   undeclared
	*/
	// get the flavor of eups to determine the path to the code
	flavorCmd := exec.Command("eups", "flavor")
	flavorBytes, _ := flavorCmd.Output()
	flavor := string(flavorBytes)
	flavor = strings.Replace(flavor, "\n", "", -1)
	rootPath := os.Getenv("EUPS_PATH")
	fullPath := rootPath + "/" + flavor + "/"
	// read the root path, and for each entry look for sub versions to remove
	dirs, err := ioutil.ReadDir(fullPath)
	if err != nil {
		fmt.Println(err)
	}
	// For each of the data products remove directories not corresponding to the version to
	// keep
	fmt.Println(len(dirs))
	for _, product := range dirs {
		versionToKeep, ok := depMap.Load(product.Name())
		if ok {
			subDirs, _ := ioutil.ReadDir(fullPath + product.Name())
			for _, version := range subDirs {
				if version.Name() != string(versionToKeep.([]byte)) {
					fmt.Println("Removing dir: ", fullPath+product.Name()+"/"+version.Name())
					rmErr := os.RemoveAll(fullPath + product.Name() + "/" + version.Name())
					if rmErr != nil {
						fmt.Println("Problem removing: ", fullPath+product.Name()+"/"+version.Name())
					}
				}
			}
		}
	}
}

func main() {
	flag.Usage = func() {
		helpString := `eupsCleanup: Undeclare and delete all products from the stack except
             those in the supplied tag and their dependancies.
             Must be run after sourceing loadLSST.<shell>

             Usage: eupsCleanup <tag>`
		fmt.Println(helpString)
		flag.PrintDefaults()
	}
	flag.Parse()

	// verify that a tag has been specified
	tagFlag := flag.Args()
	if len(tagFlag) != 1 {
		fmt.Println("A tag to keep must be specified")
		flag.Usage()
		os.Exit(1)
	}

	// Fetch all the information related to whatever the latest tag is
	eupsList := exec.Command("eups", "list", "-t", tagFlag[0])
	out, err := eupsList.Output()
	if err != nil {
		fmt.Println("Error in fetching tag list for tag: ", tagFlag[0])
		os.Exit(1)
	}
	// Split the string output into individual lines, and find the product
	// and its version. Add these to the dependency map
	var depMap sync.Map
	productList := bytes.Split(out, []byte("\n"))
	r, _ := regexp.Compile("^(\\S+)\\s*(\\S+)\\s*(.*)")
	for _, val := range productList {
		matches := r.FindAllSubmatch(val, -1)
		if len(matches) > 0 {
			depMap.Store(string(matches[0][1]), matches[0][2])
		}
	}

	fmt.Println("Checking for extra dependancies")
	updateDep(&depMap)
	fmt.Println("Undeclaring old products")
	undeclare(&depMap)
	fmt.Println("Removing old files")
	removeDirs(&depMap)
}
