#! /usr/bin/perl

#  Copyright (C) 2014 Antoine Beaupré <anarcat@anarc.at>
#  Copyright (C) 2008 Joey Schulze <joey@infodrom.org>
#
#  This program is free software; you can redistribute it and/or modify
#  it under the terms of the GNU General Public License as published by
#  the Free Software Foundation; version 2 dated June, 1991.
#
#  This program is distributed in the hope that it will be useful,
#  but WITHOUT ANY WARRANTY; without even the implied warranty of
#  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
#  GNU General Public License for more details.
#
#  You should have received a copy of the GNU General Public License
#  along with this program;  if not, write to the Free Software
#  Foundation, Inc., 59 Temple Place - Suite 330, Boston, MA 02111, USA.

# Munin plugin to monitor power usage in smart meters in the 900MHz ISM band

# Inspired by the postfix_mailstats plugin from munin-contrib

# Supported configuration:
#
#   [amr]
#   user root
#   group adm
#   env.logdir /var/log
#   env.logfile rtlamr.log

use strict;
use warnings;

use Munin::Plugin;

my $LOGDIR  = $ENV{'logdir'}  || '/var/log';
my $LOGFILE = $ENV{'logfile'} || 'rtlamr.log';
my $unit    = $ENV{'unit'}    || 'J';
if ($unit ne 'J' and $unit ne 'Wh')
{
    print "invalid unit, reverting to Joule";
    $unit = 'J';
}
my $logfile = $LOGDIR .'/'. $LOGFILE;

# station id => power consumption (in kWh)
my %stations;
# number of signals sent per station
my %signals;

sub autoconf
{
    if (-d $LOGDIR) {
	if (-f $logfile) {
            print "yes\n";
            exit 0;
	} else {
	    print "no (logfile not found)\n";
	}
    } else {
	print "no (could not find logdir)\n";
    }
    exit 1;
}

sub config
{
    print "multigraph amr_power\n";
    print "graph_title Power consumption\n";
    print "graph_args --base 1000 -l 0\n";
    print "graph_vlabel Watt\n";
    print "graph_total  Total\n";
    print "graph_category AMR\n";

    my $first = 1;
    foreach my $station (sort keys %stations) {
	my $name = clean_fieldname('station power ' . $station);
	printf "%s.label station %d\n", $name, $station;
	printf "%s.type COUNTER\n", $name;
        if ($first) {
            printf "%s.draw AREA\n", $name;
            $first = 0;
        }
        else {
            printf "%s.draw STACK\n", $name;
        }
	printf "%s.min 0\n", $name;
    }

    print "multigraph amr_meter\n";
    print "graph_title Meter reading\n";
    print "graph_args --base 1000 -l 0\n";
    print "graph_vlabel kWh\n";
    print "graph_scale  no\n";
    print "graph_category AMR\n";

    my $first = 0;
    foreach my $station (sort keys %stations) {
	my $name = clean_fieldname('station meter ' . $station);
	printf "%s.label station %d\n", $name, $station;
	printf "%s.type GAUGE\n", $name;
	printf "%s.min 0\n", $name;
    }

    print "multigraph amr_stations\n";
    print "graph_title Known AMR stations\n";
    print "graph_args --base 1000 -l 0\n";
    print "graph_vlabel stations\n";
    print "graph_category AMR\n";
    print "stations.label number of stations\n";

    print "multigraph amr_signals\n";
    print "graph_title Number of signals received\n";
    print "graph_args --base 1000 -l 0\n";
    print "graph_vlabel signals / \${graph_period}\n";
    print "graph_period minute\n";
    print "graph_category AMR\n";
    foreach my $station (sort keys %stations) {
	my $name = clean_fieldname('station signals ' . $station);
	printf "%s.label station %d\n", $name, $station;
	printf "%s.type ABSOLUTE\n", $name;
    }

    exit 0;
}

sub parse
{
    my $logfile = shift;
    my $pos = shift;

    my ($log,$rotated) = tail_open $logfile, $pos;

    while (<$log>) {
        # \d protects us against HTML injection here, be careful when changing
	if (m,SCM:{ID:(\d+) +.* +Consumption: +(\d+) +,) {
	    $stations{$1} = $2;
            $signals{$1}++;
	}
    }
    return tail_close $log;
}

need_multigraph();
autoconf if $#ARGV > -1 && $ARGV[0] eq "autoconf";

my @state_vector = restore_state;
my $pos = shift @state_vector || 0;
%stations = @state_vector;

$pos = parse $logfile, $pos;
if ($#ARGV > -1 && $ARGV[0] eq "config") {
    config;
    # don't save position on config so next run will reparse it
}
else {
    # this may not scale so well with large graphs, and is useful only
    # for debugging, when you want to run this repeatedly without
    # loosing data
    # this will also mean that stations that disappear will remain forever
    save_state $pos, %stations;
}

print "multigraph amr_power\n";
foreach my $station (sort keys %stations) {
    # the number from the counter is "Wh over 5 minutes", we want to
    # give munin joules so he can generate watts by dividing by the period
    # 1 Wh = W * 3600 s
    #      = 3600 s * J / s
    #      = 3.6 kJ
    my $power = $stations{$station} * 3600;
    if ($unit eq 'Wh') {
        # since we want absolute, we'll multiply our result by 300, the assumed time period
        $power = $stations{$station} * 300;
    }
    printf "%s.value %d\n", clean_fieldname('station power ' . $station), $power;
}

print "multigraph amr_meter\n";
foreach my $station (sort keys %stations) {
    printf "%s.value %d\n", clean_fieldname('station meter ' . $station), $stations{$station};
}

print "multigraph amr_stations\n";
printf "stations.value %d\n", scalar keys %stations;

print "multigraph amr_signals\n";
foreach my $station (sort keys %signals) {
    printf "%s.value %d\n", clean_fieldname('station signals ' . $station), $signals{$station};
}
