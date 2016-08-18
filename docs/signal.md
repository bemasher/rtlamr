---
layout: page
title: Signal Processing
redirect_from: "/2014/02/08/innards.html"
index: 3
---

{% include image.html path="/assets/signal_flow.png" caption="Overview of signal flow." %}

## Data Acquisition
***

There are two methods to get data from an rtl-sdr dongle, directly with librtlsdr and via tcp with the `rtl_tcp` spectrum server. Using librtlsdr requires the use of cgo which prevents cross-compilation; `rtl_tcp` is used instead. This has the added benefit of allowing the receiver to be somewhere other than the system running `rtlamr`.

## Demodulation
***

The ERT protocol is an on-off keyed manchester-coded signal transmitted at bit-rate of 32.768kbps. On-off keying is a type of amplitude shift keying. Individual symbols are represented as either a carrier of fixed amplitude or no transmission at all.

{% include image.html path="/assets/magnitude.png" caption="<strong>Top:</strong> Real component of complex signal. <strong>Bottom:</strong> Magnitude of complex signal. <strong>Note:</strong> Signal is truncated to show detail." %}

The signal is made up of interleaved in-phase and quadrature samples, 8-bits per component. The amplitude of each sample is:

<div>
$$\vert z\vert = \sqrt{\Re(z)^2 + \Im(z)^2}$$
</div>

To meet performance requirements the magnitude computation is done using a pre-computed lookup table which maps all possible 8-bit values to their floating-point squares. Calculating the magnitude using the lookup table then only involves two lookups, one addition and one square-root.

## Filtering
***

Filtering is required for later bit decision. The ideal filter kernel for a square-wave signal is known as a boxcar and is essentially a moving average. Due to the signal being manchester coded the ideal filter is a step function. Manchester coding mixes data with a clock signal to allow for synchronization and clock recovery while decoding as well as reducing any DC offset that would be present due to content of the data. The symbol length [[N]] is determined by the sampling rate [[F_s]] of the receiver and the data rate [[R]] of the signal.

<div>
$$N = \frac{F_s}{R}$$
</div>

The maximum sample rate without any sample loss is between 2.4 and 2.56 MHz. To simplify decoding we determine the sample rate from integral symbol lengths. From librtlsdr, the sample rate must satisfy one of two conditions:

<div>
$$
	\begin{aligned}
		225\text{kHz} \lt \, &F_s \le 300\text{kHz} \\
		900\text{kHz} \lt \, &F_s \le 3.2\text{MHz} \\
	\end{aligned}
$$
</div>

From this we can determine all of the valid symbol lengths and their corresponding sample rates. More info [here](https://github.com/bemasher/rtlamr/blob/master/help.md). Filtering can be implemented efficiently by computing the cumulative sum of each sample block and calculating the difference between a pair of subtractions:

<div>
$$
	\begin{aligned}
		\mathbf{M}_i &= \sum_{j=i}^{i+N} \mathbf{S}_j - \mathbf{S}_{j+N} \\ \\
		&= (\mathbf{C}_{i+N} - \mathbf{C}_i) - (\mathbf{C}_{i+2N} - \mathbf{C}_{i+N}) \\ \\
		&= 2\mathbf{C}_{i+N}-\mathbf{C}_{i+2 N}-\mathbf{C}_i
	\end{aligned}
$$
</div>

Where [[\mathbf{M}]] is the filtered signal, [[\mathbf{S}]] is the sample vector, [[N]] is the symbol length and [[\mathbf{C}]] is the cumulative or prefix sum of the signal.

{% include image.html path="/assets/filter.png" caption="<strong>Top:</strong> Ideal filter kernel for Manchester coded signals. <strong>Bottom:</strong> Filtered signal. <strong>Note:</strong> Signal is truncated to show detail." %}

## Bit Decision
***

The bit decision in this particular case is extremely simple. The symmetry of the filter kernel produces a signal with no DC offset, positive peaks represent a falling edge and negative peaks a rising edge. This applies at all points in the filtered signal so the bit value is only dependent on the sign of each sample.

{% include image.html path="/assets/quantized.png" caption="<strong>Red:</strong> Quantized filter signal. <strong>Blue:</strong> Filtered signal. <strong>Note:</strong> Signal is truncated to show detail." %}

## Preamble Search
***

Now that the signal has been quantized we need to search for the offset in the signal of the preamble, if there is one. The naive way to do this is simply to start at index zero and check it's value against the preamble, then if it matches look ahead one full symbol length and so on. The problem with using this method is that it has poor memory locality. For performance we interleave the signal so it can be searched linearly in memory.

## Decode
***

If a preamble is found the remaining bits following the preamble are passed to the appropriate decoder which performs a CRC to determine if the packet is valid.