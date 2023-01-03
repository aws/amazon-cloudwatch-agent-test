# install amazon EFS client on linux, agnostic to yum, apt, or zypper + rpm
# https://docs.aws.amazon.com/efs/latest/ug/installing-amazon-efs-utils.html

if command -v apt-get >/dev/null; then
  sudo apt-get update -y
  sudo apt-get -y install git binutils
  git clone https://github.com/aws/efs-utils
  cd ./efs-utils
  ./build-deb.sh
  sudo apt-get -y install ./build/amazon-efs-utils*deb
elif command -v yum >/dev/null; then
  sudo yum update -y
  sudo yum install sudo yum install -y amazon-efs-utils
elif command -v zypper >/dev/null; then
  sudo zypper refresh
  sudo zypper install -y git rpm-build make

  # redundancy to resolve known error in OpenSUSE
  # File './suse/noarch/bash-completion-2.11-2.1.noarch.rpm' not found on medium 'http://download.opensuse.org/tumbleweed/repo/oss/'
  sudo zypper ar -f -n OSS http://download.opensuse.org/tumbleweed/repo/oss/ OSS
  sudo zypper ar -f -n NON-OSS http://download.opensuse.org/tumbleweed/repo/non-oss/ NON-OSS
  sudo zypper --gpg-auto-import-keys refresh

  sudo zypper install -y git rpm-build make
  git clone https://github.com/aws/efs-utils
  cd ./efs-utils
  make rpm
  sudo zypper --no-gpg-checks install -y build/amazon-efs-utils*rpm
else
  echo "Unable to install amazon-efs-client"
  exit 1
fi