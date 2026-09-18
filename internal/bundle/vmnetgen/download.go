package vmnetgen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/bundle"
)

func download(ctx context.Context, target bundle.SocketVMNetTarget) (data []byte, failure error) {
	url := "https://github.com/lima-vm/socket_vmnet/releases/download/v" + bundle.SocketVMNetVersion + "/" + target.Filename
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", target.Filename, err)
	}
	defer func() {
		failure = errors.Join(failure, response.Body.Close())
	}()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %s", target.Filename, response.Status)
	}

	data, err = io.ReadAll(io.LimitReader(response.Body, target.Size+1))
	if err != nil {
		return nil, err
	}

	if _, err = bundle.DecodeSocketVMNet(target, data); err != nil {
		return nil, err
	}

	return data, ctx.Err()
}
