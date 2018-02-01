package main

import (
    "fmt"
    "net/http"
    "os"
    "os/exec"
    "strings"
    "io/ioutil"
    "regexp"
    "sort"
    "strconv"
    "flag"
)

func getRemoteVersion(c chan string) {
    /*
        Get the EUPS_PKGROOT, which will be the url to check for tags. The first
        elemnt in the list will be the binary url, fetch the body
    */
    pkgRootList := os.Getenv("EUPS_PKGROOT")
    pkgRootArray := strings.Split(pkgRootList, "|")
    resp, err := http.Get(pkgRootArray[0])
    if err != nil {
        fmt.Println("Problem fetching pkgroot")
        os.Exit(1)
    }
    defer resp.Body.Close()
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        fmt.Println("Problem fetching pkgroot")
        os.Exit(1)
    }

    /*
        Match the body of the response to a regular expression looking for tags
        and sort the tags acording to year and version
    */
    r, _ := regexp.Compile(">w_(\\d{4})_(\\d*).list<")
    matches := r.FindAllSubmatch(body, -1)

    sort.SliceStable(matches, func(i, j int) bool {
        f, _ := strconv.Atoi(string(matches[i][2]))
        fYear, _ := strconv.Atoi(string(matches[i][1]))
        s, _ := strconv.Atoi(string(matches[j][2]))
        sYear, _ := strconv.Atoi(string(matches[j][1]))
        if fYear > sYear {
            return true
        } else {
            return f > s
        }
    })
    // List the latest version availale
    latest := fmt.Sprintf("Latest tag available is w_%s_%s", string(matches[0][1]), string(matches[0][2]))
    c <- latest
}

func getLocalVersion(c chan string) {
    /*
        Get a list of all the local tags, and sort them by year and version
    */
    tagsCmd := exec.Command("eups", "tags")
    tagsOutput, tagsErr := tagsCmd.Output()

    if tagsErr != nil {
        fmt.Println("Problem reading local tags")
        os.Exit(1)
    }
    localR, _ := regexp.Compile("w_(\\d{4})_(\\d*)")
    localMatches := localR.FindAllSubmatch(tagsOutput, -1)

    sort.SliceStable(localMatches, func(i, j int) bool {
        f, _ := strconv.Atoi(string(localMatches[i][2]))
        fYear, _ := strconv.Atoi(string(localMatches[i][1]))
        s, _ := strconv.Atoi(string(localMatches[j][2]))
        sYear, _ := strconv.Atoi(string(localMatches[j][1]))
        if fYear > sYear {
            return true
        } else {
            return f > s
        }
    })

    // List out the latest version of the locally installed tag
    latestLocal := fmt.Sprintf("Latest tag installed is w_%s_%s", string(localMatches[0][1]), string(localMatches[0][2]))
    c <- latestLocal
}

func main() {
    flag.Usage = func() {
        helpString := `stackVersion: Check for the weekly tag version locally and remotely
              Must be run after sourceing loadLSST.<shell>`
        fmt.Println(helpString)
        flag.PrintDefaults()
    }
    flag.Parse()

    // create communication channels
    localChan := make(chan string)
    remoteChan := make(chan string)

    // go get the local version
    go getLocalVersion(localChan)
    // go get the remote version
    go getRemoteVersion(remoteChan)

    // print the versions
    fmt.Println(<-localChan)
    fmt.Println(<-remoteChan)


}
