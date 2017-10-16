platform=$(uname -s)

go build -o $(uname -s)/stackVersion ./stackVersion
go build -o $(uname -s)/eupsCleanup ./eupsCleanup
go build -o $(uname -s)/shebangtron ./shebangtron

export COPYFILE_DISABLE=1
tar -cf $platform.tar $platform
rm -r $platform
mv $platform.tar releases/
