package tunnel

import (
	"fmt"
	"testing"
)

func BenchmarkFailTrackerBlockKindPermanent(b *testing.B) {
	for _, size := range []int{1000, 100000, 500000} {
		b.Run(fmt.Sprintf("serial_%d", size), func(b *testing.B) {
			tracker, ip := benchmarkTrackerWithPermanentSet(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if tracker.blockKind(ip) != blockedPermanent {
					b.Fatal("expected permanent block")
				}
			}
		})
		b.Run(fmt.Sprintf("parallel_%d", size), func(b *testing.B) {
			tracker, ip := benchmarkTrackerWithPermanentSet(size)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if tracker.blockKind(ip) != blockedPermanent {
						b.Fatal("expected permanent block")
					}
				}
			})
		})
	}
}

func benchmarkTrackerWithPermanentSet(size int) (*failTracker, string) {
	cfg := SecurityConfig{
		AuthFailWindowSec: 300,
		AuthFailThreshold: 8,
		AuthFailBlockSec:  1800,
	}
	tracker := newFailTracker(cfg, "", "")
	target := ""
	for i := 0; i < size; i++ {
		ip := fmt.Sprintf("10.%d.%d.%d", (i>>16)&255, (i>>8)&255, i&255)
		tracker.permanent.Store(ip, struct{}{})
		target = ip
	}
	return tracker, target
}
