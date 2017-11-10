---
layout: page
title: Protocol
index: 2
---

The ERT protocol consists of several different message structures but we are only concerned with two of them. The Standard Consumption Message (SCM) and the Interval Data Message (IDM).

<div class="panel panel-default">
	<div class="panel-heading">
		<h1 class="panel-title"><strong>Standard Consumption Message (SCM)</strong></h1>
	</div>
	<div class="panel-body">
		A 12 byte message containing only an ID, commodity type, tamper flags and the current consumption value.
	</div>
	<table class="table table-hover">
		<tr>
			<th>Field</th>
			<th>Length (Bits)</th>
			<th>Description</th>
		</tr>
		<tr><td>Preamble/Sync Word</td><td>21</td><td>0x1F2A60</td></tr>
		<tr><td>Meter ID MSB</td><td>2</td><td>Two most significant bits of the ID.</td></tr>
		<tr><td>Reserved</td><td>1</td><td></td></tr>
		<tr><td>Physical Tamper Flags</td><td>2</td><td></td></tr>
		<tr>
			<td>Commodity Type</td>
			<td>4</td>
			<td>Indicates the commodity type of the meter. See list of meters: <a href="https://github.com/bemasher/rtlamr/blob/master/meters.md">meters.md</a></td>
		</tr>
		<tr><td>Encoder Tamper Flags</td><td>2</td><td></td></tr>
		<tr><td>Consumption</td><td>24</td><td>The current consumption value.</td></tr>
		<tr><td>Meter ID LSB</td><td>24</td><td>24 least significant bits of the ID.</td></tr>
		<tr><td>Checksum</td><td>16</td><td>A BCH code with generator polynomial: $p(x) = x^{16} + x^{14} + x^{13} + x^{11} + x^{10} + x^9 + x^8 + x^6 + x^5 + x + 1$</td></tr>
	</table>
</div>

<div class="panel panel-default">
	<div class="panel-heading">
		<h1 class="panel-title"><strong>Interval Data Message (IDM)</strong></h1>
	</div>
	<div class="panel-body">
		A 92 byte message containing differential consumption intervals.
	</div>
	<table class="table table-hover">
		<tr>
			<th>Field</th>
			<th>Length (Bytes)</th>
			<th>Value</th>
			<th>Description</th>
		</tr>
		<tr><td>Preamble</td><td>2</td><td>0x5555</td><td></td></tr>
		<tr><td>Sync Word</td><td>2</td><td>0x16A3</td><td></td></tr>
		<tr><td>Packet Type</td><td>1</td><td>0x1C</td><td></td></tr>
		<tr><td>Packet Length</td><td>1</td><td>0x5C</td><td></td></tr>
		<tr><td>Hamming Code</td><td>1</td><td>0xC6</td><td>Hamming code of first byte.</td></tr>
		<tr><td>Application Version</td><td>1</td><td></td><td></td></tr>
		<tr><td>Commodity Type</td><td>1</td><td></td><td>Least significant nibble is equivalent to SCM's commodity type field.</td></tr>
		<tr><td>Meter ID</td><td>4</td><td></td><td>Equivalent to SCM's Meter ID field.</td></tr>
		<tr><td>Consumption Interval Count</td><td>1</td><td></td><td></td></tr>
		<tr><td>Module Programming State</td><td>1</td><td></td><td></td></tr>
		<tr><td>Tamper Count</td><td>6</td><td></td><td></td></tr>
		<tr><td>Async Count</td><td>2</td><td></td><td></td></tr>
		<tr><td>Power Outage Flags</td><td>6</td><td></td><td></td></tr>
		<tr><td>Last Consumption</td><td>4</td><td></td><td>Equivalent to SCM's consumption field.</td></tr>
		<tr><td>Differential Consumption</td><td>53</td><td></td><td>47 intervals of 9-bit integers.</td></tr>
		<tr><td>Transmit Time Offset</td><td>2</td><td></td><td>1/16ths of a second since the first transmission for this interval.</td></tr>
		<tr><td>Meter ID Checksum</td><td>2</td><td></td><td>CRC-16-CCITT of Meter ID.</td></tr>
		<tr><td>Packet Checksum</td><td>2</td><td></td><td>CRC-16-CCITT of packet starting at Packet Type.</td></tr>
	</table>
</div>

<div class="panel panel-default">
	<div class="panel-heading">
		<h1 class="panel-title"><strong>R900 Consumption Message</strong></h1>
	</div>
	<div class="panel-body">
		A 116 bit message containing ID, consumption, backflow and leak details.
	</div>
	<table class="table table-hover">
		<tr>
			<th>Field</th>
			<th>Length (Bits)</th>
			<th>Value</th>
			<th>Description</th>
		</tr>
		<tr><td>Preamble</td><td>32</td><td>0x0000E564</td><td></td></tr>
		<tr><td>ID</td><td>32</td><td></td><td></td></tr>
		<tr><td>Unkn1</td><td>8</td><td></td><td></td></tr>
		<tr><td>NoUse</td><td>6</td><td></td><td>Day bins of no use. <a href="https://github.com/bemasher/rtlamr/issues/29#issuecomment-97622287">See issue #29 for more details.</a></td></tr>
		<tr><td>BackFlow</td><td>6</td><td></td><td>Backflow in past 35 days, high/low.</td></tr>
		<tr><td>Consumption</td><td>24</td><td></td><td></td></tr>
		<tr><td>Unkn3</td><td>2</td><td></td><td></td></tr>
		<tr><td>Leak</td><td>4</td><td></td><td>Day bins of leak. <a href="https://github.com/bemasher/rtlamr/issues/29#issuecomment-97622287">See issue #29 for more details.</a></td></tr>
		<tr><td>LeakNow</td><td>2</td><td></td><td>Leak in past 24 hours, high/low.</td></tr>
	</table>
</div>