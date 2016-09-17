import matplotlib.pyplot as plt
import numpy as np
from scipy import signal
import math

pktlen = 38144 / 4

raw = np.memmap("sample.bin", dtype=np.uint8, offset=(17600<<1)+2048+256, mode='r')

window = raw[:pktlen].copy()
level = 127.4
iq = ((level-(window.astype(np.float64))) / level).view(np.complex128)

fig, subplots = plt.subplots(nrows=1)
fig.set_size_inches(9,9*0.6180339887)

upper_lim = 1.1
subplots.plot(np.linspace(0.0, upper_lim, 100), np.linspace(0.0, upper_lim, 100))
subplots.plot(np.linspace(0.0, upper_lim, 1000), np.sqrt(np.linspace(0, upper_lim, 1000)))
subplots.grid(axis='both')
subplots.autoscale(tight=True)

plt.savefig('sqrt.svg', dpi=72, transparent=True, bbox_inches="tight")
# plt.show()
