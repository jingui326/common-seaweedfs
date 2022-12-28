package page_writer

import (
	"github.com/seaweedfs/seaweedfs/weed/glog"
	"github.com/seaweedfs/seaweedfs/weed/util"
	"github.com/seaweedfs/seaweedfs/weed/util/mem"
	"os"
	"sync"
)

var (
	_ = PageChunk(&SwapFileChunk{})
)

type ActualChunkIndex int

type SwapFile struct {
	dir                 string
	file                *os.File
	chunkSize           int64
	chunkTrackingLock   sync.Mutex
	activeChunkCount    int
	freeActualChunkList []ActualChunkIndex
}

type SwapFileChunk struct {
	sync.RWMutex
	swapfile         *SwapFile
	usage            *ChunkWrittenIntervalList
	logicChunkIndex  LogicChunkIndex
	actualChunkIndex ActualChunkIndex
	lastModifiedTsNs int64
	//memChunk         *MemChunk
}

func NewSwapFile(dir string, chunkSize int64) *SwapFile {
	return &SwapFile{
		dir:       dir,
		file:      nil,
		chunkSize: chunkSize,
	}
}
func (sf *SwapFile) FreeResource() {
	if sf.file != nil {
		sf.file.Close()
		os.Remove(sf.file.Name())
	}
}

func (sf *SwapFile) NewTempFileChunk(logicChunkIndex LogicChunkIndex) (tc *SwapFileChunk) {
	if sf.file == nil {
		var err error
		sf.file, err = os.CreateTemp(sf.dir, "")
		if err != nil {
			glog.Errorf("create swap file: %v", err)
			return nil
		}
	}
	sf.chunkTrackingLock.Lock()
	defer sf.chunkTrackingLock.Unlock()

	sf.activeChunkCount++

	// assign a new physical chunk
	var actualChunkIndex ActualChunkIndex
	if len(sf.freeActualChunkList) > 0 {
		actualChunkIndex = sf.freeActualChunkList[0]
		sf.freeActualChunkList = sf.freeActualChunkList[1:]
	} else {
		actualChunkIndex = ActualChunkIndex(sf.activeChunkCount)
	}

	swapFileChunk := &SwapFileChunk{
		swapfile:         sf,
		usage:            newChunkWrittenIntervalList(),
		logicChunkIndex:  logicChunkIndex,
		actualChunkIndex: actualChunkIndex,
		// memChunk:         NewMemChunk(logicChunkIndex, sf.chunkSize),
	}

	// println(logicChunkIndex, "|", "++++", swapFileChunk.actualChunkIndex, swapFileChunk, sf)
	return swapFileChunk
}

func (sc *SwapFileChunk) FreeResource() {

	sc.Lock()
	defer sc.Unlock()

	sc.swapfile.chunkTrackingLock.Lock()
	defer sc.swapfile.chunkTrackingLock.Unlock()

	sc.swapfile.freeActualChunkList = append(sc.swapfile.freeActualChunkList, sc.actualChunkIndex)
	sc.swapfile.activeChunkCount--
	// println(sc.logicChunkIndex, "|", "----", sc.actualChunkIndex, sc, sc.swapfile)
}

func (sc *SwapFileChunk) WriteDataAt(src []byte, offset int64, tsNs int64) (n int) {
	sc.Lock()
	defer sc.Unlock()

	if sc.lastModifiedTsNs > tsNs {
		println("write old data2", tsNs-sc.lastModifiedTsNs, "ns")
	}
	sc.lastModifiedTsNs = tsNs

	// println(sc.logicChunkIndex, "|", tsNs, "write at", offset, len(src), sc.actualChunkIndex)

	innerOffset := offset % sc.swapfile.chunkSize
	var err error
	n, err = sc.swapfile.file.WriteAt(src, int64(sc.actualChunkIndex)*sc.swapfile.chunkSize+innerOffset)
	if err == nil {
		sc.usage.MarkWritten(innerOffset, innerOffset+int64(n), tsNs)
	} else {
		glog.Errorf("failed to write swap file %s: %v", sc.swapfile.file.Name(), err)
	}
	//sc.memChunk.WriteDataAt(src, offset, tsNs)
	return
}

func (sc *SwapFileChunk) ReadDataAt(p []byte, off int64, tsNs int64) (maxStop int64) {
	sc.RLock()
	defer sc.RUnlock()

	// println(sc.logicChunkIndex, "|", tsNs, "read at", off, len(p), sc.actualChunkIndex)

	//memCopy := make([]byte, len(p))
	//copy(memCopy, p)

	chunkStartOffset := int64(sc.logicChunkIndex) * sc.swapfile.chunkSize
	for t := sc.usage.head.next; t != sc.usage.tail; t = t.next {
		logicStart := max(off, chunkStartOffset+t.StartOffset)
		logicStop := min(off+int64(len(p)), chunkStartOffset+t.stopOffset)
		if logicStart < logicStop {
			if t.TsNs >= tsNs {
				actualStart := logicStart - chunkStartOffset + int64(sc.actualChunkIndex)*sc.swapfile.chunkSize
				if _, err := sc.swapfile.file.ReadAt(p[logicStart-off:logicStop-off], actualStart); err != nil {
					glog.Errorf("failed to reading swap file %s: %v", sc.swapfile.file.Name(), err)
					break
				}
				maxStop = max(maxStop, logicStop)
			} else {
				println("read old data2", tsNs-t.TsNs, "ns")
			}
		}
	}
	//sc.memChunk.ReadDataAt(memCopy, off, tsNs)
	//if bytes.Compare(memCopy, p) != 0 {
	//	println("read wrong data from swap file", off, sc.logicChunkIndex)
	//}
	return
}

func (sc *SwapFileChunk) IsComplete() bool {
	sc.RLock()
	defer sc.RUnlock()
	return sc.usage.IsComplete(sc.swapfile.chunkSize)
}

func (sc *SwapFileChunk) LastModifiedTsNs() int64 {
	return sc.lastModifiedTsNs
}

func (sc *SwapFileChunk) SaveContent(saveFn SaveToStorageFunc) {
	sc.RLock()
	defer sc.RUnlock()

	if saveFn == nil {
		return
	}
	// println(sc.logicChunkIndex, "|", "save")
	for t := sc.usage.head.next; t != sc.usage.tail; t = t.next {
		data := mem.Allocate(int(t.Size()))
		sc.swapfile.file.ReadAt(data, t.StartOffset+int64(sc.actualChunkIndex)*sc.swapfile.chunkSize)
		reader := util.NewBytesReader(data)
		saveFn(reader, int64(sc.logicChunkIndex)*sc.swapfile.chunkSize+t.StartOffset, t.Size(), t.TsNs, func() {
		})
		mem.Free(data)
	}

}
