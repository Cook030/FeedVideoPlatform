// Package inframedia 封装 ffprobe / ffmpeg 调用，只做媒体探测与 faststart 处理。
package inframedia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	contract "GCFeed/internal/shared/contract"
)

// FFmpegProcessor 用 ffprobe / ffmpeg 实现媒体探测与 MP4 faststart 处理。
type FFmpegProcessor struct{}

// NewFFmpegProcessor 创建基于系统 ffmpeg 工具链的处理器。
func NewFFmpegProcessor() FFmpegProcessor {
	return FFmpegProcessor{}
}

// Probe 调用 ffprobe 读取视频流信息，返回结构化探测结果。
// 校验规则由 application 层负责，这里只做探测与解析。
func (FFmpegProcessor) Probe(ctx context.Context, path string) (*contract.ProbeResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		probeCtx,
		"ffprobe",
		"-v",
		"error",
		"-show_entries",
		"stream=codec_type,codec_name,width,height:format=duration",
		"-of",
		"json",
		path,
	)
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, contract.ErrVideoToolUnavailable
		}
		return nil, contract.ErrInvalidVideoMetadata
	}

	var raw probeRaw
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, contract.ErrInvalidVideoMetadata
	}
	return buildProbeResult(&raw), nil
}

// Faststart 用 ffmpeg 把 moov 前置，让视频可以边下边播。
func (FFmpegProcessor) Faststart(ctx context.Context, path string) error {
	tmp, err := createFaststartTempFile(path)
	if err != nil {
		return contract.ErrFaststartFailed
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	ffmpegCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(
		ffmpegCtx,
		"ffmpeg",
		"-y",
		"-i",
		path,
		"-map",
		"0",
		"-c",
		"copy",
		"-movflags",
		"+faststart",
		"-f",
		"mp4",
		tmpPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return contract.ErrVideoToolUnavailable
		}
		return fmt.Errorf("%w: %s", contract.ErrFaststartFailed, strings.TrimSpace(stderr.String()))
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return contract.ErrFaststartFailed
	}
	return nil
}

type probeRaw struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type probeFormat struct {
	Duration string `json:"duration"`
}

// buildProbeResult 把 ffprobe 原始输出归一化成探测结果。
// 时长解析失败时保留 0，由 application 的规则层判定为非法元数据。
func buildProbeResult(raw *probeRaw) *contract.ProbeResult {
	result := &contract.ProbeResult{}
	if raw == nil {
		return result
	}
	if duration, err := strconv.ParseFloat(strings.TrimSpace(raw.Format.Duration), 64); err == nil {
		result.DurationSeconds = duration
	}
	for _, stream := range raw.Streams {
		codecType := strings.ToLower(strings.TrimSpace(stream.CodecType))
		codecName := strings.ToLower(strings.TrimSpace(stream.CodecName))
		switch codecType {
		case "video":
			if result.HasVideo {
				continue
			}
			result.HasVideo = true
			result.Width = stream.Width
			result.Height = stream.Height
			result.VideoCodec = codecName
		case "audio":
			if result.AudioCodec == "" {
				result.AudioCodec = codecName
			}
		}
	}
	return result
}

func createFaststartTempFile(path string) (*os.File, error) {
	targetDir := filepath.Dir(path)
	return os.CreateTemp(targetDir, "*.faststart.mp4")
}
