## RTLAMR
The purpose of this project is to provide a cheap alternative to exceedingly expensive commercial products used for receiving wireless transmissions made by smart meters operating in the 900MHz ISM band. The only hardware required is a ~$20 librtlsdr compatible DVB-T dongle, typically known as rtl-sdr's.

### Usage
Usage of recv [options]:
  -centerfreq=920299072: center frequency to receive on
  -duration=0: time to run for, 0 for infinite
  -logfile="/dev/stdout": log statement dump file
  -samplefile="NUL": received message signal dump file
  -server="127.0.0.1:1234": address or hostname of rtl_tcp instance
