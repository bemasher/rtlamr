import matplotlib.pyplot as plt
import numpy as np
from scipy import signal
import math

pktlen = 38144 / 8

raw = np.memmap("sample.bin", dtype=np.uint8, offset=(596000<<1), mode='r')

window = raw[:pktlen].copy()
level = 127.4
iq = ((level-(window.astype(np.float64))) / level).view(np.complex128)

fig, subplots = plt.subplots(nrows=2)
fig.set_size_inches(9,9*0.6180339887)

(mag_plot, spec_plot) = subplots

mag = np.abs(iq)

chip_length = 72

kernel = np.append(np.ones(chip_length), -np.ones(chip_length))
filtered = np.correlate(mag, kernel)

mag_plot.step(np.arange(kernel.size), kernel)
mag_plot.grid(axis='both')
mag_plot.autoscale(tight=True)
mag_plot.set_ylim(-1.125, 1.125)
mag_plot.set_xlim(-5, chip_length*2 + 5)
mag_plot.xaxis.set_ticks([0, 36, 72, 108, 144])

spec_plot.plot(filtered)
spec_plot.grid(axis='both')
spec_plot.autoscale(tight=True)

plt.savefig('filter.svg', dpi=72, transparent=True, bbox_inches="tight")
# plt.show()