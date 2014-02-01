// Copyright 2013 Benoît Amiaux. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package rez

import (
	"sync"
)

type Config struct {
	Depth      int
	Input      int
	Output     int
	Vertical   bool
	Interlaced bool
	Threads    int
}

type Resizer interface {
	Resize(dst, src []byte, width, height, dstPitch, srcPitch int)
}

type Scaler func(dst, src []byte, cof []int16, off []int,
	taps, width, height, dstPitch, srcPitch int)

type Context struct {
	cfg     Config
	kernels []Kernel
	scaler  Scaler
}

func getHorizontalScaler(taps int) Scaler {
	switch taps {
	case 2:
		return h8scale2
	case 4:
		return h8scale4
	case 6:
		return h8scale6
	case 8:
		return h8scale8
	case 10:
		return h8scale10
	case 12:
		return h8scale12
	}
	return h8scaleN
}

func getVerticalScaler(taps int) Scaler {
	switch taps {
	case 2:
		return v8scale2
	case 4:
		return v8scale4
	case 6:
		return v8scale6
	case 8:
		return v8scale8
	case 10:
		return v8scale10
	case 12:
		return v8scale12
	}
	return v8scaleN
}

func NewResize(cfg *Config, filter Filter) Resizer {
	ctx := Context{
		cfg: *cfg,
	}
	ctx.cfg.Depth = 8 // only 8-bit for now
	ctx.kernels = []Kernel{makeKernel(&ctx.cfg, filter, 0)}
	ctx.scaler = getHorizontalScaler(ctx.kernels[0].size)
	if cfg.Vertical {
		ctx.scaler = getVerticalScaler(ctx.kernels[0].size)
		if cfg.Interlaced {
			ctx.kernels = append(ctx.kernels, makeKernel(&ctx.cfg, filter, 1))
		}
	}
	return &ctx
}

func scaleSlice(group *sync.WaitGroup, scaler Scaler,
	dst, src []byte, cof []int16, off []int, taps, width, height, dp, sp int) {
	defer group.Done()
	scaler(dst, src, cof, off, taps, width, height, dp, sp)
}

func scaleSlices(group *sync.WaitGroup, scaler Scaler,
	vertical bool, threads, taps, width, height, dp, sp int,
	dst, src []byte, cof []int16, off []int) {
	defer group.Done()
	nh := height / threads
	if nh < 1 {
		nh = 1
	}
	dst_idx := 0
	src_idx := 0
	off_idx := 0
	cof_idx := 0
	for i := 0; i < threads; i++ {
		last := i+1 == threads
		ih := nh
		if last {
			ih = height - nh*(threads-1)
		}
		if ih == 0 {
			continue
		}
		next := width
		if vertical {
			next = ih
		}
		group.Add(1)
		go scaleSlice(group, scaler,
			dst[dst_idx:dst_idx+dp*(ih-1)+width],
			src[src_idx:],
			cof[cof_idx:cof_idx+next*taps],
			off[off_idx:off_idx+next],
			taps, width, ih, dp, sp)
		if last {
			break
		}
		dst_idx += ih * dp
		if vertical {
			cof_idx += ih * taps
			for j := 0; j < ih; j++ {
				src_idx += sp * off[off_idx+j]
			}
			off_idx += ih
		} else {
			src_idx += sp * ih
		}
	}
}

func (c *Context) Resize(dst, src []byte, width, height, dp, sp int) {
	field := bin(c.cfg.Vertical && c.cfg.Interlaced)
	dwidth := c.cfg.Output
	dheight := height
	if c.cfg.Vertical {
		dwidth = width
		dheight = c.cfg.Output >> field
	}
	group := sync.WaitGroup{}
	for i, k := range c.kernels[:1+field] {
		group.Add(1)
		go scaleSlices(&group, c.scaler, c.cfg.Vertical, c.cfg.Threads,
			k.size, dwidth, dheight, dp<<field, sp<<field,
			dst[dp*i:], src[sp*i:], k.coeffs, k.offsets)
	}
	group.Wait()
}
