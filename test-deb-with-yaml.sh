export BINARY_SOURCE=release/mw-agent
export RELEASE_VERSION=1.0.0
export CONTROL_FILE=control
export ARCH=amd64
echo "release-version"
echo $RELEASE_VERSION


mkdir -p example/apt-repo
mkdir -p example/mw-agent_$RELEASE_VERSION-1_$ARCH/usr/bin
mkdir -p example/mw-agent_$RELEASE_VERSION-1_$ARCH/DEBIAN
mkdir -p controlsetup
touch controlsetup/control

cat << EOF > controlsetup/control
Package: mw-agent
Version: ${RELEASE_VERSION}
Maintainer: middleware <dev@middleware.io>
Depends: libc6
Architecture: $ARCH
Homepage: https://middleware.io
Description: Middleware Agent
EOF


cp $BINARY_SOURCE example/mw-agent_$RELEASE_VERSION-1_$ARCH/usr/bin/.
# cp $BINARY_SOURCE example/mw-agent_$RELEASE_VERSION-1_$ARCH/etc/mw-agent/.

cp controlsetup/$CONTROL_FILE example/mw-agent_$RELEASE_VERSION-1_$ARCH/DEBIAN/control

mkdir -p example/mw-agent_$RELEASE_VERSION-1_$ARCH/etc/mw-agent/configyamls/all
mkdir -p example/mw-agent_$RELEASE_VERSION-1_$ARCH/etc/mw-agent/configyamls/nodocker
cp configyamls/all/otel-config.yaml example/mw-agent_$RELEASE_VERSION-1_$ARCH/etc/mw-agent/configyamls/all/
cp configyamls/nodocker/otel-config.yaml example/mw-agent_$RELEASE_VERSION-1_$ARCH/etc/mw-agent/configyamls/nodocker/

dpkg --build example/mw-agent_$RELEASE_VERSION-1_$ARCH
dpkg-deb --info example/mw-agent_$RELEASE_VERSION-1_$ARCH.deb
dpkg-deb --contents example/mw-agent_$RELEASE_VERSION-1_$ARCH.deb