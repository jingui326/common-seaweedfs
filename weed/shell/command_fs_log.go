package shell

import (
	"flag"
	"fmt"
	"github.com/seaweedfs/seaweedfs/weed/filer"
	"github.com/seaweedfs/seaweedfs/weed/pb/filer_pb"
	"io"
	"time"
)

func init() {
	Commands = append(Commands, &commandFsLogPurge{})
}

type commandFsLogPurge struct {
}

func (c *commandFsLogPurge) Name() string {
	return "fs.log.purge"
}

func (c *commandFsLogPurge) Help() string {
	return `purge filer logs

	fs.log.purge [-v] [-modifyDayAgo 365]
`
}

func (c *commandFsLogPurge) Do(args []string, commandEnv *CommandEnv, writer io.Writer) (err error) {
	fsLogPurgeCommand := flag.NewFlagSet(c.Name(), flag.ContinueOnError)
	modifyDayAgo := fsLogPurgeCommand.Uint("modifyDayAgo", 365, "purge logs older than N days")
	verbose := fsLogPurgeCommand.Bool("v", false, "verbose mode")

	if err = fsLogPurgeCommand.Parse(args); err != nil {
		return err
	}
	modificationTimeAgo := time.Now().Add(-time.Hour * 24 * time.Duration(*modifyDayAgo)).Unix()
	err = filer_pb.ReadDirAllEntries(commandEnv, filer.SystemLogDir, "", func(entry *filer_pb.Entry, isLast bool) error {
		if entry.Attributes.Mtime > modificationTimeAgo {
			return nil
		}
		if errDel := filer_pb.Remove(commandEnv, filer.SystemLogDir, entry.Name, true, true, true, false, nil); errDel != nil {
			return errDel
		}
		if *verbose {
			fmt.Fprintf(writer, "delete %s\n", entry.Name)
		}
		return nil
	})
	return err
}
