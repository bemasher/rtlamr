import matplotlib.pyplot as plt
import numpy as np
from scipy import signal
import math

pktlen = 38144 / 8

raw = np.memmap("sample.bin", dtype=np.uint8, offset=(596000<<1), mode='r')

window = raw[:pktlen].copy()
iq = ((127.5-(window.astype(np.float64))) / 127.5).view(np.complex128)

fig, subplots = plt.subplots(nrows=2)
fig.set_size_inches(9,9*0.6180339887)

(mag_plot, spec_plot) = subplots

mag_plot.plot(iq.real, linewidth=0.5)
mag_plot.grid(axis='both')
mag_plot.autoscale(tight=True)

mag = np.abs(iq)
spec_plot.plot(mag, linewidth=0.5)
spec_plot.grid(axis='both')
spec_plot.autoscale(tight=True)

# plt.show()
fig.savefig('magnitude.svg', dpi=72, transparent=True, bbox_inches="tight")