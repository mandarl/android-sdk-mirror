#!/bin/bash

if [ -z "$1" ]
  then
    echo "ERR! No version number supplied"
    exit
fi

version=$1

#current directory name is app name
appname=${PWD##*/}
echo $appname

#run the default task to publish to github
sed -i "s/appnameplaceholder/$appname/g" .goxc.json
goxc -d=/tmp -c=, -pv=$version

#generate binaries
sed -i "s/appnameplaceholder/$appname/g" binaries.goxc.json
sed -i "s/appnameplaceholder/$appname/g" main.go
goxc -c=binaries -pv=$version


#remove any .gz files from previous runs; go-selfupdate processes these files
#   as binary and creates .gz.gz file
rm $GOPATH/bin/$appname-xc/$appname/$version/*.gz*
rm $GOPATH/bin/$appname-xc/$appname/*.json
#generate selfupdate archives and json
go-selfupdate -o $GOPATH/bin/$appname-xc/$appname/ $GOPATH/bin/$appname-xc/$version/$appname/ $version


#upload to server
lftp -e "set ftp:ssl-allow no;open -u usrdist,$DISTPWD ftp.dipoletech.com;mirror -c -R $GOPATH/bin/$appname-xc/$appname /$appname;bye"

