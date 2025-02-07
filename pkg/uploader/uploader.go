package uploader

import (
	"context"
	"errors"
	"github.com/fatih/color"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/pkg/prog"
	"github.com/iyear/tdl/pkg/utils"
	"github.com/jedib0t/go-pretty/v6/progress"
	"golang.org/x/sync/errgroup"
	"io"
	"time"
)

var formatter = utils.Byte.FormatBinaryBytes

type Uploader struct {
	client   *tg.Client
	pw       progress.Writer
	partSize int
	threads  int
	iter     Iter
}

func New(client *tg.Client, partSize int, threads int, iter Iter) *Uploader {
	return &Uploader{
		client:   client,
		pw:       prog.New(formatter),
		partSize: partSize,
		threads:  threads,
		iter:     iter,
	}
}

func (u *Uploader) Upload(ctx context.Context, limit int) error {
	u.pw.Log(color.GreenString("All files will be uploaded to 'Saved Messages' dialog"))

	u.pw.SetNumTrackersExpected(u.iter.Total(ctx))

	go u.pw.Render()

	wg, errctx := errgroup.WithContext(ctx)
	wg.SetLimit(limit)

	go runPS(errctx, u.pw)

	for u.iter.Next(ctx) {
		item, err := u.iter.Value(ctx)
		if err != nil {
			u.pw.Log(color.RedString("Get item failed: %v, skip...", err))
			continue
		}

		wg.Go(func() error {
			// d.pw.Log(color.MagentaString("name: %s,size: %s", item.Name, utils.Byte.FormatBinaryBytes(item.Size)))
			return u.upload(errctx, item)
		})
	}

	err := wg.Wait()
	if err != nil {
		u.pw.Stop()
		for u.pw.IsRenderInProgress() {
			time.Sleep(time.Millisecond * 10)
		}

		if errors.Is(err, context.Canceled) {
			color.Red("Upload aborted.")
		}
		return err
	}

	for u.pw.IsRenderInProgress() {
		if u.pw.LengthActive() == 0 {
			u.pw.Stop()
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

func (u *Uploader) upload(ctx context.Context, item *Item) error {
	defer func(R io.ReadCloser) {
		_ = R.Close()
	}(item.R)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	tracker := prog.AppendTracker(u.pw, formatter, item.Name, item.Size)

	up := uploader.NewUploader(u.client).
		WithPartSize(u.partSize).WithThreads(u.threads).WithProgress(&_progress{tracker: tracker})

	f, err := up.Upload(ctx, uploader.NewUpload(item.Name, item.R, item.Size))
	if err != nil {
		return err
	}

	doc := message.UploadedDocument(f,
		styling.Code(item.Name),
		styling.Plain(" - "),
		styling.Code(item.MIME),
	).MIME(item.MIME).Filename(item.Name)

	var media message.MediaOption = doc

	if utils.Media.IsVideo(item.MIME) {
		media = doc.Video().SupportsStreaming()
	}
	if utils.Media.IsAudio(item.MIME) {
		media = doc.Audio().Title(utils.FS.GetNameWithoutExt(item.Name))
	}

	_, err = message.NewSender(u.client).WithUploader(up).Self().Media(ctx, media)
	return err
}
