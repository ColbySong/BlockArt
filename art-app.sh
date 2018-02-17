echo Usage: sh art-app.sh [miner num] [art-app num]

minerNum=$1
appNum=$2

cd miner-$minerNum

pwd

go run art-app-$appNum.go
