echo Usage: sh generate-canvas.sh [miner num]

minerNum=$1

cd miner-$minerNum

pwd

go run generate-canvas.go

echo Canvas:

cat canvas.html

echo Opening canvas in browser...

open canvas.html