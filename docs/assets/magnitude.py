import matplotlib.pyplot as plt
import numpy as np
from scipy import signal
import math

pktlen = 38144 / 8

raw = np.memmap("sample.bin", dtype=np.uint8, offset=(17600<<1)+2048+256, mode='r')

window = raw[:pktlen].copy()
level = 127.4
iq = ((level-(window.astype(np.float64))) / level).view(np.complex128)

fig, subplots = plt.subplots(nrows=2)
fig.set_size_inches(9,9*0.6180339887)

(mag_plot, spec_plot) = subplots

mag_plot.plot(iq, linewidth=0.5)
mag_plot.grid(axis='both')
mag_plot.autoscale(tight=True)

spec_plot.plot(np.abs(iq), linewidth=0.5)
spec_plot.grid(axis='both')
spec_plot.autoscale(tight=True)

# plt.show()
fig.savefig('magnitude.png', dpi=96, transparent=True, bbox_inches="tight")