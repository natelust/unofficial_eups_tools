platform=$(uname -s)

url=https://raw.githubusercontent.com/natelust/unofficial_eups_tools/master/releases/$platform.tar
installPath=$EUPS_DIR/bin/

curl -sL $url | tar -xf - -C $installPath --strip-components=1
rm $installPath/platform
