#!/bin/bash
if [ -z "$1" ]
then
      echo "please pass target as 1st argument"
      exit
fi

if [ -z "$2" ]
then
      echo "please pass image name as 2nd argument"
      exit
fi

if [ -z "$3" ]
then
      echo "please pass release tag as 3rd argument"
      exit
fi

docker build .  --target $1 -t $2:$3 -f $4

