package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const csvFile = "diagnostics.csv"

type diagnostic struct {
	StartTimestamp string
	StartUnix      int64
	Duration       string
	ResourceGroups int
	API            string
}

func (d *diagnostic) toCSVString() string {
	return strings.Join(
		[]string{
			d.StartTimestamp,
			strconv.FormatInt(d.StartUnix, 10),
			d.Duration,
			strconv.Itoa(d.ResourceGroups),
			d.API,
		}, ",")
}

func (d *diagnostic) write() {
	line := d.toCSVString()

	file, err := os.OpenFile(csvFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Printf("error writing csv file '%s': %s\n", csvFile, err.Error())
		return
	}
	defer file.Close()

	file.WriteString(line)
	file.WriteString("\n")
}

func newDiagnostic(start time.Time, resourceGroups int, api string) {
	d := &diagnostic{
		StartTimestamp: start.Format("2006-01-02 15:04:05"),
		StartUnix:      start.Unix(),
		Duration:       time.Since(start).String(),
		ResourceGroups: resourceGroups,
		API:            api,
	}

	time.Since(start)
	d.write()
}
