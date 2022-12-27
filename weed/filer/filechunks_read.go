package filer

import (
	"container/list"
	"github.com/seaweedfs/seaweedfs/weed/pb/filer_pb"
	"golang.org/x/exp/slices"
)

func readResolvedChunks(chunks []*filer_pb.FileChunk) (visibles []VisibleInterval) {

	var points []*Point
	for _, chunk := range chunks {
		points = append(points, &Point{
			x:       chunk.Offset,
			ts:      chunk.ModifiedTsNs,
			chunk:   chunk,
			isStart: true,
		})
		points = append(points, &Point{
			x:       chunk.Offset + int64(chunk.Size),
			ts:      chunk.ModifiedTsNs,
			chunk:   chunk,
			isStart: false,
		})
	}
	slices.SortFunc(points, func(a, b *Point) bool {
		if a.x != b.x {
			return a.x < b.x
		}
		if a.ts != b.ts {
			return a.ts < b.ts
		}
		return !a.isStart
	})

	var prevX int64
	queue := list.New() // points with higher ts are at the tail
	var lastPoint *Point
	for _, point := range points {
		if queue.Len() > 0 {
			lastPoint = queue.Back().Value.(*Point)
		} else {
			lastPoint = nil
		}
		if point.isStart {
			if lastPoint != nil {
				if point.x != prevX && lastPoint.ts < point.ts {
					visibles = addToVisibles(visibles, prevX, lastPoint, point)
					prevX = point.x
				}
			}
			// insert into queue
			if lastPoint == nil || lastPoint.ts < point.ts {
				queue.PushBack(point)
				prevX = point.x
			} else {
				for e := queue.Front(); e != nil; e = e.Next() {
					if e.Value.(*Point).ts > point.ts {
						queue.InsertBefore(point, e)
						break
					}
				}
			}
		} else {
			var isLast bool
			for e := queue.Back(); e != nil; e = e.Prev() {
				isLast = e.Next() == nil
				if e.Value.(*Point).ts == point.ts {
					queue.Remove(e)
					break
				}
			}
			if isLast && lastPoint != nil {
				visibles = addToVisibles(visibles, prevX, lastPoint, point)
				prevX = point.x
			}
		}
	}

	return
}

func addToVisibles(visibles []VisibleInterval, prevX int64, startPoint *Point, point *Point) []VisibleInterval {
	if prevX < point.x {
		chunk := startPoint.chunk
		visibles = append(visibles, VisibleInterval{
			start:        prevX,
			stop:         point.x,
			fileId:       chunk.GetFileIdString(),
			modifiedTsNs: chunk.ModifiedTsNs,
			chunkOffset:  prevX - chunk.Offset,
			chunkSize:    chunk.Size,
			cipherKey:    chunk.CipherKey,
			isGzipped:    chunk.IsCompressed,
		})
	}
	return visibles
}

type Point struct {
	x       int64
	ts      int64
	chunk   *filer_pb.FileChunk
	isStart bool
}
