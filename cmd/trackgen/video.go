package main

import (
	"bytes"
	"fmt"
	"image"
	"os/exec"
	"strconv"

	"github.com/mariuspot/f1mcp/internal/tracks"
)

// haveFFmpeg reports whether ffmpeg is installed; videos are skipped
// without it.
func haveFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// writeFollowVideo renders the follow-camera animation of laps into an MP4
// file, streaming frames straight into ffmpeg.
func writeFollowVideo(file string, t *tracks.Track, laps []tracks.LapTrace, o tracks.Options) error {
	w, h := tracks.FollowSize()
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", w, h),
		"-r", strconv.FormatFloat(tracks.FollowFPS, 'f', -1, 64), "-i", "-",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "26", "-preset", "medium",
		"-movflags", "+faststart", file)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	renderErr := tracks.RenderLapFollow(t, laps, o, func(frame *image.RGBA) error {
		_, err := in.Write(frame.Pix)
		return err
	})
	in.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg: %v: %s", err, stderr.String())
	}
	return renderErr
}
