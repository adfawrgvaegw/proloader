package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	buildingpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/building"
	reqpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/request"
	respb "github.com/Yeah114/Fatalder/cmd/grpc/pb/response"
	"github.com/Yeah114/Fatalder/utils/data_manager"
)

type BuildingService struct {
	buildingpb.UnimplementedBuildingServiceServer
	dm *data_manager.DataManager
}

func NewBuildingService(dm *data_manager.DataManager) *BuildingService {
	return &BuildingService{dm: dm}
}

func (s *BuildingService) UploadBuilding(stream buildingpb.BuildingService_UploadBuildingServer) error {
	var (
		filename string
		final    string
		temp     string
		file     *os.File
	)

	cleanup := func() {
		if file != nil {
			file.Close()
			file = nil
		}
		if temp != "" {
			_ = os.Remove(temp)
		}
	}

	sendError := func(msg string) error {
		cleanup()
		return stream.SendAndClose(&respb.BoolResponse{Error: msg})
	}

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			cleanup()
			return err
		}
		if chunk == nil {
			continue
		}

		if filename == "" {
			filename = strings.TrimSpace(chunk.GetFilename())
			if filename == "" {
				return sendError("filename required")
			}
			path, err := s.dm.PrepareBuildingFilePath(filename)
			if err != nil {
				return sendError(err.Error())
			}
			final = path
			temp = final + ".upload"
			file, err = os.Create(temp)
			if err != nil {
				return sendError(err.Error())
			}
		} else if name := strings.TrimSpace(chunk.GetFilename()); name != "" && name != filename {
			return sendError("filename mismatch in stream")
		}

		if file == nil {
			return sendError("filename must be provided before data")
		}

		if len(chunk.GetContent()) == 0 {
			continue
		}

		if _, err := file.Write(chunk.GetContent()); err != nil {
			return sendError(fmt.Sprintf("write chunk failed: %v", err))
		}
	}

	if filename == "" {
		return sendError("no data received")
	}

	if file != nil {
		if err := file.Sync(); err != nil {
			return sendError(err.Error())
		}
		if err := file.Close(); err != nil {
			return sendError(err.Error())
		}
		file = nil
	}

	if temp != "" {
		if err := os.Rename(temp, final); err != nil {
			if os.IsExist(err) {
				if removeErr := os.Remove(final); removeErr != nil && !os.IsNotExist(removeErr) {
					return sendError(fmt.Sprintf("cleanup existing file failed: %v", removeErr))
				}
				if renameErr := os.Rename(temp, final); renameErr != nil {
					return sendError(fmt.Sprintf("finalize upload failed: %v", renameErr))
				}
			} else {
				return sendError(fmt.Sprintf("finalize upload failed: %v", err))
			}
		}
		temp = ""
	}

	return stream.SendAndClose(&respb.BoolResponse{Result: true})
}

const downloadChunkSize = 64 * 1024

func (s *BuildingService) DownloadBuilding(req *respb.StringResponse, stream buildingpb.BuildingService_DownloadBuildingServer) error {
	if err := stream.Context().Err(); err != nil {
		return err
	}
	name := strings.TrimSpace(req.GetResult())
	if name == "" {
		return fmt.Errorf("filename required")
	}

	path, err := s.dm.PrepareBuildingFilePath(name)
	if err != nil {
		return err
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file %s not found", name)
		}
		return err
	}
	defer file.Close()

	buf := make([]byte, downloadChunkSize)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			if err := stream.Context().Err(); err != nil {
				return err
			}
			chunk := &buildingpb.BuildingChunk{
				Filename: name,
				Content:  append([]byte(nil), buf[:n]...),
			}
			if err := stream.Send(chunk); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (s *BuildingService) ListBuildingInfo(ctx context.Context, _ *reqpb.EmptyRequest) (*buildingpb.ListBuildingInfoResponse, error) {
	resp := &buildingpb.ListBuildingInfoResponse{}
	files, err := s.dm.ListBuildingFiles()
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}

	for _, info := range files {
		resp.Info = append(resp.Info, toBuildingInfo(&info))
	}
	return resp, nil
}

func (s *BuildingService) GetBuildingInfo(ctx context.Context, req *buildingpb.GetBuildingInfoRequest) (*buildingpb.GetBuildingInfoResponse, error) {
	resp := &buildingpb.GetBuildingInfoResponse{}
	name := strings.TrimSpace(req.GetFilename())
	if name == "" {
		resp.Error = "filename required"
		return resp, nil
	}

	info, ok, err := s.dm.GetBuildingInfo(name)
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}
	if !ok {
		resp.Error = "file not found"
		return resp, nil
	}

	resp.Info = toBuildingInfo(info)
	return resp, nil
}

func (s *BuildingService) DeleteBuilding(ctx context.Context, req *buildingpb.DeleteBuildingRequest) (*respb.StringResponse, error) {
	name := strings.TrimSpace(req.GetFilename())
	if name == "" {
		return &respb.StringResponse{Error: "filename required"}, nil
	}

	if err := s.dm.DeleteBuilding(name); err != nil {
		if os.IsNotExist(err) {
			return &respb.StringResponse{Result: fmt.Sprintf("building %s not found", name)}, nil
		}
		return &respb.StringResponse{Error: err.Error()}, nil
	}

	return &respb.StringResponse{Result: fmt.Sprintf("deleted building %s", name)}, nil
}

func (s *BuildingService) RenameBuilding(ctx context.Context, req *respb.StringResponse) (*respb.StringResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimSpace(req.GetResult()), "|")
	if len(parts) != 2 {
		return &respb.StringResponse{Error: "payload must be 'old|new'"}, nil
	}
	oldName := strings.TrimSpace(parts[0])
	newName := strings.TrimSpace(parts[1])
	if oldName == "" || newName == "" {
		return &respb.StringResponse{Error: "both old and new filenames required"}, nil
	}
	oldPath, err := s.dm.PrepareBuildingFilePath(oldName)
	if err != nil {
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	newPath, err := s.dm.PrepareBuildingFilePath(newName)
	if err != nil {
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	return &respb.StringResponse{Result: fmt.Sprintf("renamed %s to %s", oldName, newName)}, nil
}

func toBuildingInfo(info *data_manager.BuildingFileInfo) *buildingpb.BuildingInfo {
	if info == nil {
		return nil
	}
	ts := info.ModTime
	if ts.IsZero() {
		ts = time.Now()
	}
	return &buildingpb.BuildingInfo{
		Filename:   info.Name,
		Size:       info.Size,
		Md5:        info.MD5,
		CreateTime: ts.Unix(),
	}
}
