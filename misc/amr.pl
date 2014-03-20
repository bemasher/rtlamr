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
my $logfile = $LOGDIR .'/'. $LOGFILE;

# station id => power consumption (in kWh)
my %stations;

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
    print "graph_title Power consumption\n";
    print "graph_args --base 1000 -l 0\n";
    print "graph_vlabel kWh\n";
    print "graph_scale  no\n";
    print "graph_total  Total\n";
    print "graph_category sensors\n";

    my $first = 0;
    foreach my $station (sort keys %stations) {
	my $name = clean_fieldname('station ' . $station);
	printf "%s.label station %d\n", $name, $station;
	printf "%s.type GAUGE\n", $name;
        if ($first) {
            printf "%s.draw AREA\n", $name;
        }
        else {
            printf "%s.draw STACK\n", $name;
        }
	printf "%s.min 0\n", $name;
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
	}
    }
    return tail_close $log;
}

autoconf if $#ARGV > -1 && $ARGV[0] eq "autoconf";

my @state_vector = restore_state;
my $pos = shift @state_vector || 0;

if ($#ARGV > -1 && $ARGV[0] eq "config") {
    config $logfile, $pos;
    # don't save position on config so next run will reparse it
}
else {
    $pos = parse $logfile, $pos;
    save_state $pos;
}

foreach my $station (sort keys %stations) {
    printf "%s.value %d\n", clean_fieldname('station ' . $station), $stations{$station};
}
