# crush-cli

Command line app for recompressing JPEGS with very little perceived loss in quality.

Crush is a Go app to recompress JPEGS. It uses the [jpeg-archive](https://github.com/danielgtaylor/jpeg-archive) binary and runs things concurrently and in parallel. Note - this does not strip out the metadata in the JPEG, which is perfect for my use case.

## Goals

I want a simple binary that I can copy around and use. `jpeg-archive` works good at processing a single JPG, but it stores it to a new file. I want to overwrite the original JPEG.

I would also like to see about using `cgo` to statically link the algorithms in `jpeg-archive` and `MozJPEG`.
