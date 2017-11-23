A word about the `-symbollength` flag:

`symbollength` sets the symbol length in samples. Defaults to 72.

Sample rate is determined by this value as follows:

```
DataRate = 32768
SampleRate = SymbolLength * DataRate
```

Sample rates are limited by the dongle such that:

```
225 kHz < Sample Rate < 300 kHz and
900 kHz < Sample Rate < 3.2 MHz
```

The symbol length along with the message type determines the size of sample blocks read and processed on each pass.

PreambleSymbols = 21 (for SCM) and 32 (for IDM)

```go
BlockSize = NextPowerOf2(d.Cfg.PreambleLength)

func NextPowerOf2(v int) int {
	return 1 << uint(math.Ceil(math.Log2(float64(v))))
}
```

Valid symbol lengths are given below, with a block size calculated for SCM:

| Symbol Length | Sample Rate | Block Size |
| ------------- | ----------- | ---------- |
| 7             | 229.376 kHz | 512        |
| 8             | 262.144 kHz | 512        |
| 9             | 294.912 kHz | 512        |

| Symbol Length | Sample Rate  | Block Size | Symbol Length | Sample Rate  | Block Size |
| ------------- | ------------ | ---------  | ------------- | -----------  | ---------- |
| 28            | 917.504 kHz  | 2048       | 39            | 1.277952 MHz | 2048       |
| 29            | 950.272 kHz  | 2048       | 40            | 1.310720 MHz | 2048       |
| 30            | 983.040 kHz  | 2048       | 41            | 1.343488 MHz | 2048       |
| 31            | 1.015808 MHz | 2048       | 42            | 1.376256 MHz | 2048       |
| 32            | 1.048576 MHz | 2048       | 43            | 1.409024 MHz | 2048       |
| 33            | 1.081344 MHz | 2048       | 44            | 1.441792 MHz | 2048       |
| 34            | 1.114112 MHz | 2048       | 45            | 1.474560 MHz | 2048       |
| 35            | 1.146880 MHz | 2048       | 46            | 1.507328 MHz | 2048       |
| 36            | 1.179648 MHz | 2048       | 47            | 1.540096 MHz | 2048       |
| 37            | 1.212416 MHz | 2048       | 48            | 1.572864 MHz | 2048       |
| 38            | 1.245184 MHz | 2048       |

| Symbol Length | Sample Rate  | Block Size | Symbol Length | Sample Rate  | Block Size |
| ------------- | -----------  | ---------- | ------------- | -----------  | ---------- |
| 49            | 1.605632 MHz | 4096       | 74            | 2.424832 MHz | 4096       |
| 50            | 1.638400 MHz | 4096       | 75            | 2.457600 MHz | 4096       |
| 51            | 1.671168 MHz | 4096       | 76            | 2.490368 MHz | 4096       |
| 52            | 1.703936 MHz | 4096       | 77            | 2.523136 MHz | 4096       |
| 53            | 1.736704 MHz | 4096       | 78            | 2.555904 MHz | 4096       |
| 54            | 1.769472 MHz | 4096       | 79            | 2.588672 MHz | 4096       |
| 55            | 1.802240 MHz | 4096       | 80            | 2.621440 MHz | 4096       |
| 56            | 1.835008 MHz | 4096       | 81            | 2.654208 MHz | 4096       |
| 57            | 1.867776 MHz | 4096       | 82            | 2.686976 MHz | 4096       |
| 58            | 1.900544 MHz | 4096       | 83            | 2.719744 MHz | 4096       |
| 59            | 1.933312 MHz | 4096       | 84            | 2.752512 MHz | 4096       |
| 60            | 1.966080 MHz | 4096       | 85            | 2.785280 MHz | 4096       |
| 61            | 1.998848 MHz | 4096       | 86            | 2.818048 MHz | 4096       |
| 62            | 2.031616 MHz | 4096       | 87            | 2.850816 MHz | 4096       |
| 63            | 2.064384 MHz | 4096       | 88            | 2.883584 MHz | 4096       |
| 64            | 2.097152 MHz | 4096       | 89            | 2.916352 MHz | 4096       |
| 65            | 2.129920 MHz | 4096       | 90            | 2.949120 MHz | 4096       |
| 66            | 2.162688 MHz | 4096       | 91            | 2.981888 MHz | 4096       |
| 67            | 2.195456 MHz | 4096       | 92            | 3.014656 MHz | 4096       |
| 68            | 2.228224 MHz | 4096       | 93            | 3.047424 MHz | 4096       |
| 69            | 2.260992 MHz | 4096       | 94            | 3.080192 MHz | 4096       |
| 70            | 2.293760 MHz | 4096       | 95            | 3.112960 MHz | 4096       |
| 71            | 2.326528 MHz | 4096       | 96            | 3.145728 MHz | 4096       |
| 72            | 2.359296 MHz | 4096       | 97            | 3.178496 MHz | 4096       |
| 73            | 2.392064 MHz | 4096       |
