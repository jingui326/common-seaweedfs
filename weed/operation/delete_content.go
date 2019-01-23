package operation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chrislusf/seaweedfs/weed/pb/volume_server_pb"
)

type DeleteResult struct {
	Fid    string `json:"fid"`
	Size   int    `json:"size"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

func ParseFileId(fid string) (vid string, keyCookie string, err error) {
	commaIndex := strings.Index(fid, ",")
	if commaIndex <= 0 {
		return "", "", errors.New("Wrong fid format.")
	}
	return fid[:commaIndex], fid[commaIndex+1:], nil
}

// DeleteFiles batch deletes a list of fileIds
func DeleteFiles(master string, fileIds []string) ([]*volume_server_pb.DeleteResult, error) {

	lookupFunc := func(vids []string) (map[string]LookupResult, error) {
		return LookupVolumeIds(master, vids)
	}

	return DeleteFilesWithLookupVolumeId(fileIds, lookupFunc)

}

func DeleteFilesWithLookupVolumeId(fileIds []string, lookupFunc func(vid []string) (map[string]LookupResult, error)) ([]*volume_server_pb.DeleteResult, error) {

	var ret []*volume_server_pb.DeleteResult

	vidToFileids := make(map[string][]string)
	var vids []string
	for _, fileId := range fileIds {
		vid, _, err := ParseFileId(fileId)
		if err != nil {
			ret = append(ret, &volume_server_pb.DeleteResult{
				FileId: vid,
				Status: http.StatusBadRequest,
				Error:  err.Error()},
			)
			continue
		}
		if _, ok := vidToFileids[vid]; !ok {
			vidToFileids[vid] = make([]string, 0)
			vids = append(vids, vid)
		}
		vidToFileids[vid] = append(vidToFileids[vid], fileId)
	}

	lookupResults, err := lookupFunc(vids)
	if err != nil {
		return ret, err
	}

	serverToFileids := make(map[string][]string)
	for vid, result := range lookupResults {
		if result.Error != "" {
			ret = append(ret, &volume_server_pb.DeleteResult{
				FileId: vid,
				Status: http.StatusBadRequest,
				Error:  err.Error()},
			)
			continue
		}
		for _, location := range result.Locations {
			if _, ok := serverToFileids[location.Url]; !ok {
				serverToFileids[location.Url] = make([]string, 0)
			}
			serverToFileids[location.Url] = append(
				serverToFileids[location.Url], vidToFileids[vid]...)
		}
	}

	var wg sync.WaitGroup

	for server, fidList := range serverToFileids {
		wg.Add(1)
		go func(server string, fidList []string) {
			defer wg.Done()

			if deleteResults, deleteErr := DeleteFilesAtOneVolumeServer(server, fidList); deleteErr != nil {
				err = deleteErr
			} else {
				ret = append(ret, deleteResults...)
			}

		}(server, fidList)
	}
	wg.Wait()

	return ret, err
}

// DeleteFilesAtOneVolumeServer deletes a list of files that is on one volume server via gRpc
func DeleteFilesAtOneVolumeServer(volumeServer string, fileIds []string) (ret []*volume_server_pb.DeleteResult, err error) {

	err = WithVolumeServerClient(volumeServer, func(volumeServerClient volume_server_pb.VolumeServerClient) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
		defer cancel()

		req := &volume_server_pb.BatchDeleteRequest{
			FileIds: fileIds,
		}

		resp, err := volumeServerClient.BatchDelete(ctx, req)

		// fmt.Printf("deleted %v %v: %v\n", fileIds, err, resp)

		if err != nil {
			return err
		}

		ret = append(ret, resp.Results...)

		return nil
	})

	if err != nil {
		return
	}

	for _, result := range ret {
		if result.Error != "" && result.Error != "not found" {
			return nil, fmt.Errorf("delete fileId %s: %v", result.FileId, result.Error)
		}
	}

	return

}
