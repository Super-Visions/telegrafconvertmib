#!/bin/bash

# First argument is configuration location
Dest=$(realpath $1)
shift

# Walk other arguments and check if the files contain traps
Files=()
for File
do
	if grep -q "NOTIFICATION-TYPE\|TRAP-TYPE" "$File"; then
		Files+=($(realpath $File))
	fi
done

# Iterate the loop to read and print each array element
cd $(dirname $0)
for File in "${Files[@]}"
do
	./telegrafconvertmib -p $(dirname $File) -m $(basename $File) -d $Dest
done