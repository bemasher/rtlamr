import matplotlib.pyplot as plt
import numpy as np
from scipy import signal
import math

pktlen = 38144 / 4

raw = np.memmap("sample.bin", dtype=np.uint8, offset=(596000<<1), mode='r')

window = raw[:pktlen].copy()
iq = ((127.5-(window.astype(np.float64))) / 127.5).view(np.complex128)

fig, subplots = plt.subplots(nrows=1)
fig.set_size_inches(9,9*0.6180339887 / 2)

(mag_plot) = subplots

mag = np.abs(iq)

kernel = np.append(np.ones(72), -np.ones(72))
filtered = np.correlate(mag, kernel)

mag_plot.plot(filtered / filtered.max())
filtered = np.digitize(filtered, [0]) * 2 - 1
mag_plot.plot(filtered, color="red")

mag_plot.grid(axis='both')
mag_plot.autoscale(tight=True)
mag_plot.set_ylim(-1.25, 1.25)

plt.savefig('quantized.svg', dpi=72, transparent=True, bbox_inches="tight")
# plt.show()
