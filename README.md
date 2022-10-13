My Toy Lang
===============

Go port of [LLVM's Kaleidoscope Tutorial](http://llvm.org/docs/tutorial/LangImpl1.html) using the tinygo llvm bindings. <sup>[doc](https://pkg.go.dev/tinygo.org/x/go-llvm)</sup>


Compile and Run
===============
```bash
# Ubuntu 20.04 Golang 16.0+
LLVM_SOURCE="deb https://mirrors.tuna.tsinghua.edu.cn/llvm-apt/focal/ llvm-toolchain-focal-14 main"
sudo echo "$LLVM_SOURCE" > /etc/apt/sources.list.d/llvm.list
sudo apt update
sudo apt install llvm-14-dev

go build -ldflags "-s -w"
./toy fib.k && ./fib
```


Other Resources
===============

* [LLVM's Official C++ Kaleidoscope Tutorial](http://llvm.org/docs/tutorial/LangImpl1.html)

* [Rob Pike's *Lexical Scanning in Go*](http://www.youtube.com/watch?v=HxaD_trXwRE) — our lexer is based on the design outlined in this talk.

* [Go bindings to a system-installed LLVM](https://github.com/tinygo-org/go-llvm)

* [LLVM's Kaleidoscope in Golang](https://github.com/ripta/kaleidoscope)

* [Golang Port of LLVM Kaleidoscope](https://github.com/ajsnow/kaleidoscope)
