package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	pkg  = flag.String("pkg", "", "package to test")
	cmd  = flag.String("cmd", "go test -v", "command to run")
	vers = flag.String("tags", "1.7,1.8,1.9,latest", "comma-delimited versions to test")
	dimg = flag.String("name", "golang", "docker image name to use")
	logf = flag.String("file", "", "file to write rsync/Docker output to (stdout, stderr accepted)")

	cmdOut io.Writer
)

const (
	dockerTmpl = `FROM golang:%[1]s
COPY %[3]s %[3]s
RUN cd %[3]s && %[4]s`
)

func main() {
	flag.Usage = func() {
		const usage = `multi-test: test multiple Go version and GOARCH combinations.

Usage:
`
		fmt.Fprintf(os.Stderr, "%s\n", usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	switch *logf {
	case "":
		cmdOut = ioutil.Discard
	case "stdout":
		cmdOut = os.Stdout
	case "stderr":
		cmdOut = os.Stderr
	default:
		file, err := os.Create(*logf)
		if err != nil {
			log.Fatalf("error opening logf: %s\n", err)
		}
		cmdOut = file
	}

	if *pkg == "" {
		log.Fatal("pkg flag must be set")
	}

	lpath := filepath.Join("src", *pkg)
	fpath := filepath.Join(os.Getenv("GOPATH"), lpath)
	if _, err := os.Stat(fpath); os.IsNotExist(err) {
		log.Fatalf("could not find pkg: %q", fpath)
	}

	goVers := strings.Split(*vers, ",")
	if len(goVers) == 0 {
		log.Fatal("must provide at least 1 Go version to test")
	}

	tempd, err := ioutil.TempDir("", "_testdir")
	if err != nil {
		log.Fatalf("could not create tempdir: %s\n", err)
	}
	defer os.RemoveAll(tempd)

	if err := os.Chdir(tempd); err != nil {
		errorf("could not chdir to %q: %s\n", tempd, err)
		return
	}

	if err := os.MkdirAll(lpath, 0777); err != nil {
		errorf("could not mkdirall %q: %s\n", lpath, err)
		return
	}

	cleanup, err := loadFiles(lpath, fpath)
	if err != nil {
		errorf("loading files failed: %s\n", err)
		return
	}
	defer func() { cleanup() }()

	file, err := ioutil.TempFile(".", "Dockerfile")
	if err != nil {
		errorf("creation of a tempfile failed: %s\n", err)
		return
	}
	defer file.Close()

	dfile := file.Name()
	base := filepath.Base(lpath)
	for _, v := range goVers {
		err = overwrite(file, fmt.Sprintf(dockerTmpl, v, base, lpath, *cmd))
		if err != nil {
			errorf("(re-)writing Dockerfile failed: %s\n", err)
			return
		}
		buildcmd := fmt.Sprintf(`set -euo pipefail
			     docker build -f %[1]s -t %[2]s .
				 docker run --rm %[2]s
				 docker rmi   -f %[2]s`,
			dfile,
			fmt.Sprintf("multi-test:go%s", v),
		)
		if err := run("sh", "-c", buildcmd); err != nil {
			errorf("docker build/run/rmi failed: %s\n", err)
			return
		}
	}
}

func run(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout = cmdOut
	c.Stderr = cmdOut
	return c.Run()
}

func overwrite(file *os.File, content string) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, os.SEEK_SET); err != nil {
		return err
	}
	_, err := file.WriteString(content)
	return err
}

func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}