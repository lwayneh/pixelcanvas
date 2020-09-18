package pixelcanvas

import (
	"syscall/js"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/pixelgl"
)

// Canvasp is used to store all variables needed share info between js and go
type Canvasp struct {
	done chan struct{} // Used as part of 'run forever' in the render handler

	// DOM properties
	window js.Value
	doc    js.Value
	body   js.Value

	// Canvas properties
	canvas  js.Value
	ctx     js.Value
	imgData js.Value
	width   int
	height  int

	// Drawing Context
	image    *pixelgl.Canvas // The Shadow frame we actually draw on
	reqID    js.Value        // Storage of the current annimationFrame requestID - For Cancel
	timeStep float64         // Min Time delay between frames. - Calculated as   maxFPS/1000

	copybuff js.Value
}

// RenderFunc passes canvas drawing calls to/from go
type RenderFunc func(gc *pixelgl.Canvas) bool

// NewCanvasp Creates a new Canvasp
func NewCanvasp(create bool) (*Canvasp, error) {

	var c Canvasp

	c.window = js.Global()
	c.doc = c.window.Get("document")
	c.body = c.doc.Get("body")

	// If create, make a canvas that fills the windows
	if create {
		c.Create(int(c.window.Get("innerWidth").Int()), int(c.window.Get("innerHeight").Int()))
	}

	return &c, nil
}

// Create a new Canvas in the DOM, and append it to the Body.
// This also calls Set to create relevant shadow Buffer etc
func (c *Canvasp) Create(width int, height int) {

	// Make the Canvas
	canvas := c.doc.Call("createElement", "canvas")

	canvas.Set("height", height)
	canvas.Set("width", width)
	c.body.Call("appendChild", canvas)

	c.Set(canvas, width, height)
}

// Set is used to setup with an existing Canvas element which was obtained from JS
func (c *Canvasp) Set(canvas js.Value, width int, height int) {
	c.canvas = canvas
	c.height = height
	c.width = width

	// Setup the 2D Drawing context
	c.ctx = c.canvas.Call("getContext", "2d")
	c.imgData = c.ctx.Call("createImageData", width, height) // Note Width, then Height
	c.image = pixelgl.NewCanvas(pixel.R(0, 0, float64(width), float64(height)))
	c.copybuff = js.Global().Get("Uint8Array").New(len(c.image.Pixels())) // Static JS buffer for copying data out to JS. Defined once and re-used to save on un-needed allocations

}

// Start starts the annimationFrame callbacks running.
func (c *Canvasp) Start(maxFPS float64, rf RenderFunc) {
	c.SetFPS(maxFPS)
	c.initFrameUpdate(rf)
}

// Stop needs to be called on an 'beforeUnload' trigger,
// to properly close out the render callback, and prevent
// browser errors on page Refresh
func (c *Canvasp) Stop() {
	c.window.Call("cancelAnimationFrame", c.reqID)
	c.done <- struct{}{}
	close(c.done)
}

// SetFPS Sets the maximum FPS (Frames per Second).  This can be changed
// on the fly and will take affect next frame.
func (c *Canvasp) SetFPS(maxFPS float64) {
	c.timeStep = 1000 / maxFPS
}

// Height returns CanvasP height
func (c *Canvasp) Height() int {
	return c.height
}

// Width returns CanvasP width
func (c *Canvasp) Width() int {
	return c.width
}

// initFrameUpdate copies the image over to the browser
func (c *Canvasp) initFrameUpdate(rf RenderFunc) {
	// Hold the callbacks without blocking
	go func() {
		var renderFrame js.Func
		var lastTimestamp float64

		renderFrame = js.FuncOf(func(this js.Value, args []js.Value) interface{} {

			timestamp := args[0].Float()
			if timestamp-lastTimestamp >= c.timeStep { // Constrain FPS

				if rf != nil { // If required, call the requested render function, before copying the frame
					if rf(c.image) { // Only copy the image back if RenderFunction returns TRUE. (i.e. stuff has changed.)
						c.imgCopy()
					}
				} else { // Just do the copy, rendering must be being done elsewhere
					c.imgCopy()
				}

				lastTimestamp = timestamp
			}

			c.reqID = js.Global().Call("requestAnimationFrame", renderFrame) // Captures the requestID to be used in Close / Cancel
			return nil
		})
		defer renderFrame.Release()
		js.Global().Call("requestAnimationFrame", renderFrame)
		<-c.done
	}()
}

// imgCopy Does the actuall copy over of the image data for the 'render' call.
func (c *Canvasp) imgCopy() {
	js.CopyBytesToJS(c.copybuff, c.image.Pixels())
	c.imgData.Get("data").Call("set", c.copybuff)
	c.ctx.Call("putImageData", c.imgData, 0, 0)
}
