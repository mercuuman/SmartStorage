package garbage

import (
	"context"
	"time"
)

func StartGC(ctx context.Context, gc *Service) {

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = gc.Run(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}
