package main

import (
	"errors"
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
	logf = flag.String("file", "", "file to write Docker output to (stdout, stderr accepted)")

	cmdOut io.Writer
)

func main() {
	if err := Main(); err != nil {
		log.Fatal(err)
	}
}

func Main() error {
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
			return fmt.Errorf("error opening logf: %s\n", err)
		}
		defer file.Close()
		cmdOut = file
	}

	if *pkg == "" {
		return errors.New("pkg flag must be set")
	}

	pkgPath := filepath.Join("src", *pkg)
	absPath := filepath.Join(os.Getenv("GOPATH"), pkgPath)

	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("could not find pkg: %q", absPath)
		}
		return fmt.Errorf("error calling stat: %s", err)
	}

	tags := strings.Split(*vers, ",")
	if len(tags) == 0 {
		log.Fatal("must provide at least 1 Go version to test")
	}

	tempd, err := ioutil.TempDir("", "_testdir")
	if err != nil {
		return fmt.Errorf("could not create tempdir: %s\n", err)
	}
	// Don't worry about removing child files as they'll get nuked when tempd
	// does.
	defer os.RemoveAll(tempd)

	if err := os.Chdir(tempd); err != nil {
		return fmt.Errorf("could not chdir to %q: %s\n", tempd, err)
	}

	// e.g., src/github.com/acmecorp/widget
	if err := os.MkdirAll(pkgPath, 0777); err != nil {
		return fmt.Errorf("could not mkdirall %q: %s\n", pkgPath, err)
	}

	cleanup, err := loadFiles(pkgPath, absPath)
	if err != nil {
		return fmt.Errorf("loading files failed: %s\n", err)
	}
	defer cleanup()

	dfile, err := ioutil.TempFile(".", "Dockerfile")
	if err != nil {
		return fmt.Errorf("creation of a tempfile failed: %s\n", err)
	}
	defer dfile.Close()

	dfname := dfile.Name()
	for _, tag := range tags {
		err = writeDockerfile(dfile, *dimg, tag, pkgPath, *cmd)
		if err != nil {
			return fmt.Errorf("(re-)writing Dockerfile (%q) failed: %s\n", dfname, err)
		}

		const commandTmpl = `set -e
docker build -f %[1]s -t %[2]s .
docker run --rm %[2]s
docker rmi   -f %[2]s`

		img := fmt.Sprintf("multi-test:%s-%s", *dimg, tag) // image name
		cmd := fmt.Sprintf(commandTmpl, dfname, img)       // shell command
		if err := run("sh", "-c", cmd); err != nil {
			return fmt.Errorf("docker build/run/rmi failed: %s\n", err)
		}
	}
	return nil
}

func run(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout = cmdOut
	c.Stderr = cmdOut
	return c.Run()
}

func writeDockerfile(file *os.File, image, tag, path, cmd string) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, os.SEEK_SET); err != nil {
		return err
	}
	// TODO: The RUN command assumes we're in $GOPATH.
	const template = `FROM %[1]s:%[2]s
COPY %[3]s %[3]s
RUN cd %[3]s && %[4]s`
	_, err := file.WriteString(fmt.Sprintf(template, image, tag, path, cmd))
	return err
}
