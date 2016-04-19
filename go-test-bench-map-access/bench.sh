#!/bin/sh

# go test -test.benchmem -test.benchtime=5s -test.count=1 -test.cpuprofile cpu.out -test.memprofile mem.out -o bin.out -test.run=^$ -test.bench=.
# go test -test.benchmem -test.benchtime=5s -test.count=1 -test.cpuprofile maprange.cpu.out -test.memprofile maprange.mem.out -o maprange.bin.out -test.run=^$ -test.bench=BenchmarkMaprange
# -run=^$ to disable test

# benchmark one by one

if [ -z "$1" ]
then
        echo "USAGE: $0 <xxx_test.go>"
        exit 1
fi

list=''
if [ "$2" = 'all' ]
then
        list='all'
else
        list=`cat $1 | grep 'func Benchmark'|tr '(' ' ' | awk '{print $2}'`
fi

for one in $list; 
do 
	echo " --- $one"
        selname="$one"
        test "$one" = 'all' && selname='.'
        go test -test.run=^$ -test.benchmem -test.benchtime=5s -test.count=1 -test.cpuprofile cpu.pprof.$one -test.memprofile mem.pprof.$one -o bin.out.$one -test.bench=$selname; 
	echo '------'; 
        echo "go tool pprof bin.out.$one ./cpu.pprof.$one"
        echo "go tool pprof bin.out.$one ./mem.pprof.$one"
	echo '------'; 
	sleep 1; 
done
