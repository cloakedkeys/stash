package manager

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/remeh/sizedwaitgroup"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/scene/generate"
	"github.com/stashapp/stash/pkg/sliceutil"
	"github.com/stashapp/stash/pkg/sliceutil/stringslice"
)

type GenerateMetadataInput struct {
	Covers              bool                         `json:"covers"`
	Sprites             bool                         `json:"sprites"`
	Previews            bool                         `json:"previews"`
	ImagePreviews       bool                         `json:"imagePreviews"`
	PreviewOptions      *GeneratePreviewOptionsInput `json:"previewOptions"`
	Markers             bool                         `json:"markers"`
	MarkerImagePreviews bool                         `json:"markerImagePreviews"`
	MarkerScreenshots   bool                         `json:"markerScreenshots"`
	Transcodes          bool                         `json:"transcodes"`
	// Generate transcodes even if not required
	ForceTranscodes           bool `json:"forceTranscodes"`
	Phashes                   bool `json:"phashes"`
	ImagePhashes              bool `json:"imagePhashes"`
	InteractiveHeatmapsSpeeds bool `json:"interactiveHeatmapsSpeeds"`
	ClipPreviews              bool `json:"clipPreviews"`
	ImageThumbnails           bool `json:"imageThumbnails"`
	// scene ids to generate for
	SceneIDs []string `json:"sceneIDs"`
	// marker ids to generate for
	MarkerIDs []string `json:"markerIDs"`
	// image ids to generate for
	ImageIDs []string `json:"imageIDs"`
	// gallery ids to generate for
	GalleryIDs []string `json:"galleryIDs"`
	// overwrite existing media
	Overwrite bool `json:"overwrite"`
	// paths to run generate on, in addition to the other ID lists
	Paths []string `json:"paths"`
}

type GeneratePreviewOptionsInput struct {
	// Number of segments in a preview file
	PreviewSegments *int `json:"previewSegments"`
	// Preview segment duration, in seconds
	PreviewSegmentDuration *float64 `json:"previewSegmentDuration"`
	// Duration of start of video to exclude when generating previews
	PreviewExcludeStart *string `json:"previewExcludeStart"`
	// Duration of end of video to exclude when generating previews
	PreviewExcludeEnd *string `json:"previewExcludeEnd"`
	// Preset when generating preview
	PreviewPreset *models.PreviewPreset `json:"previewPreset"`
}

const generateQueueSize = 200000

type GenerateJob struct {
	repository models.Repository
	input      GenerateMetadataInput

	overwrite      bool
	fileNamingAlgo models.HashAlgorithm

	totals totalsGenerate
}

type totalsGenerate struct {
	covers                   int64
	sprites                  int64
	previews                 int64
	imagePreviews            int64
	markers                  int64
	transcodes               int64
	phashes                  int64
	imagePhashes             int64
	interactiveHeatmapSpeeds int64
	clipPreviews             int64
	imageThumbnails          int64

	tasks int
}

func (j *GenerateJob) Execute(ctx context.Context, progress *job.Progress) error {
	var err error

	j.overwrite = j.input.Overwrite
	j.fileNamingAlgo = config.GetInstance().GetVideoFileNamingAlgorithm()

	config := config.GetInstance()
	parallelTasks := config.GetParallelTasksWithAutoDetection()

	logger.Infof("Generate started with %d parallel tasks", parallelTasks)

	queue := make(chan Task, generateQueueSize)
	go func() {
		defer close(queue)

		g := &generate.Generator{
			Encoder:      instance.FFMpeg,
			FFProbe:      instance.FFProbe,
			FFMpegConfig: instance.Config,
			LockManager:  instance.ReadLockManager,
			MarkerPaths:  instance.Paths.SceneMarkers,
			ScenePaths:   instance.Paths.Scene,
			Overwrite:    j.overwrite,
		}

		r := j.repository
		if err := r.WithReadTxn(ctx, func(ctx context.Context) error {
			qb := r.Scene
			if len(j.input.SceneIDs) == 0 &&
				len(j.input.MarkerIDs) == 0 &&
				len(j.input.ImageIDs) == 0 &&
				len(j.input.GalleryIDs) == 0 &&
				len(j.input.Paths) == 0 {

				j.queueTasks(ctx, g, nil, queue)
			} else {
				t, err := j.findTargets(ctx)
				if err != nil {
					return err
				}

				for _, s := range t.scenes {
					if err := s.LoadFiles(ctx, qb); err != nil {
						return err
					}

					j.queueSceneJobs(ctx, g, s, queue)
				}

				for _, m := range t.markers {
					j.queueMarkerJob(g, m, queue)
				}

				for _, i := range t.images {
					if err := i.LoadFiles(ctx, r.Image); err != nil {
						return err
					}

					j.queueImageJob(g, i, queue)
				}

				if len(j.input.Paths) > 0 {
					paths := filterStashPaths(j.input.Paths)
					j.queueTasks(ctx, g, paths, queue)
				}
			}

			return nil
		}); err != nil && ctx.Err() == nil {
			logger.Error(err.Error())
			return
		}

		totals := j.totals
		logMsg := "Generating"
		if j.input.Covers {
			logMsg += fmt.Sprintf(" %d covers", totals.covers)
		}
		if j.input.Sprites {
			logMsg += fmt.Sprintf(" %d sprites", totals.sprites)
		}
		if j.input.Previews {
			logMsg += fmt.Sprintf(" %d previews", totals.previews)
		}
		if j.input.ImagePreviews {
			logMsg += fmt.Sprintf(" %d image previews", totals.imagePreviews)
		}
		if j.input.Markers {
			logMsg += fmt.Sprintf(" %d markers", totals.markers)
		}
		if j.input.Transcodes {
			logMsg += fmt.Sprintf(" %d transcodes", totals.transcodes)
		}
		if j.input.Phashes {
			logMsg += fmt.Sprintf(" %d phashes", totals.phashes)
		}
		if j.input.ImagePhashes {
			logMsg += fmt.Sprintf(" %d image phashes", totals.imagePhashes)
		}
		if j.input.InteractiveHeatmapsSpeeds {
			logMsg += fmt.Sprintf(" %d heatmaps & speeds", totals.interactiveHeatmapSpeeds)
		}
		if j.input.ClipPreviews {
			logMsg += fmt.Sprintf(" %d image clip previews", totals.clipPreviews)
		}
		if j.input.ImageThumbnails {
			logMsg += fmt.Sprintf(" %d image thumbnails", totals.imageThumbnails)
		}
		if logMsg == "Generating" {
			logMsg = "Nothing selected to generate"
		}
		logger.Infof(logMsg)

		progress.SetTotal(int(totals.tasks))
	}()

	wg := sizedwaitgroup.New(parallelTasks)

	// Start measuring how long the generate has taken. (consider moving this up)
	start := time.Now()
	if err = instance.Paths.Generated.EnsureTmpDir(); err != nil {
		logger.Warnf("could not create temporary directory: %v", err)
	}

	defer func() {
		if err := instance.Paths.Generated.EmptyTmpDir(); err != nil {
			logger.Warnf("failure emptying temporary directory: %v", err)
		}
	}()

	for f := range queue {
		if job.IsCancelled(ctx) {
			// keep draining the queue so the producer goroutine can finish
			// and release its read transaction, otherwise the DB stays locked
			continue
		}

		wg.Add()
		// #1879 - need to make a copy of f - otherwise there is a race condition
		// where f is changed when the goroutine runs
		localTask := f
		go progress.ExecuteTask(localTask.GetDescription(), func() {
			localTask.Start(ctx)
			wg.Done()
			progress.Increment()
		})
	}

	wg.Wait()

	if job.IsCancelled(ctx) {
		logger.Info("Stopping due to user request")
		return nil
	}

	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("Generate finished (%s)", elapsed))
	return nil
}

func (j *GenerateJob) queueTasks(ctx context.Context, g *generate.Generator, paths []string, queue chan<- Task) {
	j.totals = totalsGenerate{}

	j.queueScenesTasks(ctx, g, paths, queue)
	j.queueImagesTasks(ctx, g, paths, queue)
}

func (j *GenerateJob) queueScenesTasks(ctx context.Context, g *generate.Generator, paths []string, queue chan<- Task) {
	const batchSize = 1000

	findFilter := models.BatchFindFilter(batchSize)
	sceneFilter := scene.FilterFromPaths(paths)

	r := j.repository

	for more := true; more; {
		if job.IsCancelled(ctx) {
			return
		}

		scenes, err := scene.Query(ctx, r.Scene, sceneFilter, findFilter)
		if err != nil {
			logger.Errorf("Error encountered queuing files to scan: %s", err.Error())
			return
		}

		for _, ss := range scenes {
			if job.IsCancelled(ctx) {
				return
			}

			if err := ss.LoadFiles(ctx, r.Scene); err != nil {
				logger.Errorf("Error encountered queuing files to scan: %s", err.Error())
				return
			}

			j.queueSceneJobs(ctx, g, ss, queue)
		}

		if len(scenes) != batchSize {
			more = false
		} else {
			*findFilter.Page++
		}
	}
}

func (j *GenerateJob) queueImagesTasks(ctx context.Context, g *generate.Generator, paths []string, queue chan<- Task) {
	const batchSize = 1000

	findFilter := models.BatchFindFilter(batchSize)
	imageFilter := image.FilterFromPaths(paths)

	r := j.repository

	for more := j.input.ClipPreviews || j.input.ImageThumbnails || j.input.ImagePhashes; more; {
		if job.IsCancelled(ctx) {
			return
		}

		images, err := image.Query(ctx, r.Image, imageFilter, findFilter)
		if err != nil {
			logger.Errorf("Error encountered queuing files to scan: %s", err.Error())
			return
		}

		for _, ss := range images {
			if job.IsCancelled(ctx) {
				return
			}

			if err := ss.LoadFiles(ctx, r.Image); err != nil {
				logger.Errorf("Error encountered queuing files to scan: %s", err.Error())
				return
			}

			j.queueImageJob(g, ss, queue)
		}

		if len(images) != batchSize {
			more = false
		} else {
			*findFilter.Page++
		}
	}
}

func getGeneratePreviewOptions(optionsInput GeneratePreviewOptionsInput) generate.PreviewOptions {
	config := config.GetInstance()

	ret := generate.PreviewOptions{
		Segments:        config.GetPreviewSegments(),
		SegmentDuration: config.GetPreviewSegmentDuration(),
		ExcludeStart:    config.GetPreviewExcludeStart(),
		ExcludeEnd:      config.GetPreviewExcludeEnd(),
		Preset:          config.GetPreviewPreset().String(),
		Audio:           config.GetPreviewAudio(),
	}

	if optionsInput.PreviewSegments != nil {
		ret.Segments = *optionsInput.PreviewSegments
	}

	if optionsInput.PreviewSegmentDuration != nil {
		ret.SegmentDuration = *optionsInput.PreviewSegmentDuration
	}

	if optionsInput.PreviewExcludeStart != nil {
		ret.ExcludeStart = *optionsInput.PreviewExcludeStart
	}

	if optionsInput.PreviewExcludeEnd != nil {
		ret.ExcludeEnd = *optionsInput.PreviewExcludeEnd
	}

	if optionsInput.PreviewPreset != nil {
		ret.Preset = optionsInput.PreviewPreset.String()
	}

	return ret
}

func (j *GenerateJob) queueSceneJobs(ctx context.Context, g *generate.Generator, scene *models.Scene, queue chan<- Task) {
	r := j.repository

	if j.input.Covers {
		task := &GenerateCoverTask{
			repository: r,
			Scene:      *scene,
			Overwrite:  j.overwrite,
		}

		if task.required(ctx) {
			j.totals.covers++
			j.totals.tasks++
			queue <- task
		}
	}

	if j.input.Sprites {
		task := &GenerateSpriteTask{
			Scene:               *scene,
			Overwrite:           j.overwrite,
			fileNamingAlgorithm: j.fileNamingAlgo,
		}

		if task.required() {
			j.totals.sprites++
			j.totals.tasks++
			queue <- task
		}
	}

	generatePreviewOptions := j.input.PreviewOptions
	if generatePreviewOptions == nil {
		generatePreviewOptions = &GeneratePreviewOptionsInput{}
	}
	options := getGeneratePreviewOptions(*generatePreviewOptions)

	if j.input.Previews {
		task := &GeneratePreviewTask{
			Scene:               *scene,
			VideoPreview:        true,
			ImagePreview:        j.input.ImagePreviews,
			Options:             options,
			Overwrite:           j.overwrite,
			fileNamingAlgorithm: j.fileNamingAlgo,
			generator:           g,
		}

		if task.required() {
			if task.videoPreviewRequired() {
				j.totals.previews++
			}
			if task.imagePreviewRequired() {
				j.totals.imagePreviews++
			}

			j.totals.tasks++
			queue <- task
		}
	}

	if j.input.Markers || j.input.MarkerImagePreviews || j.input.MarkerScreenshots {
		task := &GenerateMarkersTask{
			repository:          r,
			Scene:               scene,
			Overwrite:           j.overwrite,
			fileNamingAlgorithm: j.fileNamingAlgo,
			VideoPreview:        j.input.Markers,
			ImagePreview:        j.input.MarkerImagePreviews,
			Screenshot:          j.input.MarkerScreenshots,

			generator: g,
		}

		markers := task.markersNeeded(ctx)
		if markers > 0 {
			j.totals.markers += int64(markers)
			j.totals.tasks++

			queue <- task
		}
	}

	if j.input.Transcodes {
		forceTranscode := j.input.ForceTranscodes
		task := &GenerateTranscodeTask{
			Scene:               *scene,
			Overwrite:           j.overwrite,
			Force:               forceTranscode,
			fileNamingAlgorithm: j.fileNamingAlgo,
			g:                   g,
		}
		if task.required() {
			j.totals.transcodes++
			j.totals.tasks++
			queue <- task
		}
	}

	if j.input.Phashes {
		// generate for all files in scene
		for _, f := range scene.Files.List() {
			task := &GeneratePhashTask{
				repository:          r,
				File:                f,
				fileNamingAlgorithm: j.fileNamingAlgo,
				Overwrite:           j.overwrite,
			}

			if task.required() {
				j.totals.phashes++
				j.totals.tasks++
				queue <- task
			}
		}
	}

	if j.input.InteractiveHeatmapsSpeeds {
		task := &GenerateInteractiveHeatmapSpeedTask{
			repository:          r,
			Scene:               *scene,
			Overwrite:           j.overwrite,
			fileNamingAlgorithm: j.fileNamingAlgo,
		}

		if task.required() {
			j.totals.interactiveHeatmapSpeeds++
			j.totals.tasks++
			queue <- task
		}
	}
}

func (j *GenerateJob) queueMarkerJob(g *generate.Generator, marker *models.SceneMarker, queue chan<- Task) {
	task := &GenerateMarkersTask{
		repository:          j.repository,
		Marker:              marker,
		Overwrite:           j.overwrite,
		fileNamingAlgorithm: j.fileNamingAlgo,
		VideoPreview:        j.input.Markers,
		ImagePreview:        j.input.MarkerImagePreviews,
		Screenshot:          j.input.MarkerScreenshots,
		generator:           g,
	}
	j.totals.markers++
	j.totals.tasks++
	queue <- task
}

func (j *GenerateJob) queueImageJob(g *generate.Generator, image *models.Image, queue chan<- Task) {
	if j.input.ImageThumbnails {
		task := &GenerateImageThumbnailTask{
			Image:     *image,
			Overwrite: j.overwrite,
		}

		if task.required() {
			j.totals.imageThumbnails++
			j.totals.tasks++
			queue <- task
		}
	}

	if j.input.ClipPreviews {
		task := &GenerateClipPreviewTask{
			Image:     *image,
			Overwrite: j.overwrite,
		}

		if task.required() {
			j.totals.clipPreviews++
			j.totals.tasks++
			queue <- task
		}
	}

	if j.input.ImagePhashes {
		// generate for all files in image
		for _, f := range image.Files.List() {
			task := &GenerateImagePhashTask{
				repository: j.repository,
				File:       f,
				Overwrite:  j.overwrite,
			}

			if task.required() {
				j.totals.imagePhashes++
				j.totals.tasks++
				queue <- task
			}
		}
	}
}

// generateIDs converts the ids of items to generate for, logging any that
// are not numbers. An id listed more than once is kept once, so each item is
// generated for once.
func generateIDs(ids []string) []int {
	ret, err := stringslice.StringSliceToIntSlice(ids)
	if err != nil {
		logger.Error(err.Error())
	}

	return sliceutil.Unique(ret)
}

// uniqueIDStrings keeps each id once, in the order given, for tasks that
// take ids as text. Ids that are the same number written differently, such
// as "1" and "01", count as the same, as in generateIDs. Anything else, such
// as "*" or a value that isn't a number, is kept as written so the task
// handles it as before.
func uniqueIDStrings(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	ret := make([]string, 0, len(ids))
	for _, id := range ids {
		key := id
		if n, err := strconv.Atoi(id); err == nil {
			key = strconv.Itoa(n)
		}

		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ret = append(ret, id)
	}

	return ret
}

// generateTargets are the items named by the id lists of a generate input.
type generateTargets struct {
	scenes  []*models.Scene
	markers []*models.SceneMarker
	// images, followed by the images of the galleries
	images []*models.Image
}

// findTargets finds the items named by the id lists of the input. Items that
// no longer exist are skipped, with a warning naming them.
func (j *GenerateJob) findTargets(ctx context.Context) (*generateTargets, error) {
	r := j.repository
	ret := &generateTargets{}
	var err error

	if ret.scenes, err = findExisting(ctx, "scene", generateIDs(j.input.SceneIDs), r.Scene.FindMany, r.Scene.Find); err != nil {
		return nil, err
	}

	if ret.markers, err = findExisting(ctx, "marker", generateIDs(j.input.MarkerIDs), r.SceneMarker.FindMany, r.SceneMarker.Find); err != nil {
		return nil, err
	}

	if ret.images, err = findExisting(ctx, "image", generateIDs(j.input.ImageIDs), r.Image.FindMany, r.Image.Find); err != nil {
		return nil, err
	}

	galleries, err := findExisting(ctx, "gallery", generateIDs(j.input.GalleryIDs), r.Gallery.FindMany, r.Gallery.Find)
	if err != nil {
		return nil, err
	}

	for _, g := range galleries {
		imgs, err := r.Image.FindByGalleryID(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		ret.images = append(ret.images, imgs...)
	}

	return ret, nil
}

// findExisting finds the items with the given ids, in order. FindMany fails
// if any item is missing, so on failure each item is looked up on its own:
// missing items are skipped and named in a warning, and any other error is
// returned.
func findExisting[T any](ctx context.Context, kind string, ids []int,
	findMany func(context.Context, []int) ([]*T, error),
	find func(context.Context, int) (*T, error),
) ([]*T, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	ret, findManyErr := findMany(ctx, ids)
	if findManyErr == nil {
		return ret, nil
	}

	ret = nil
	var missing []string
	for _, id := range ids {
		v, err := find(ctx, id)
		if err != nil {
			// FindMany's error comes first: on PostgreSQL a failed query
			// aborts the transaction, so this error only says that
			return nil, fmt.Errorf("%w (looking up %s %d after that: %w)", findManyErr, kind, id, err)
		}

		if v == nil {
			missing = append(missing, strconv.Itoa(id))
			continue
		}

		ret = append(ret, v)
	}

	if len(missing) == 0 {
		// every item exists, so FindMany failed for another reason
		return nil, findManyErr
	}

	logger.Warnf("Generate: skipping %s ids that no longer exist: %s", kind, strings.Join(missing, ", "))
	return ret, nil
}
