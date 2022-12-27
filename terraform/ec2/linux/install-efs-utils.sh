# install amazon EFS client on linux, agnostic to yum or apt
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
else
echo "Unable to install amazon-efs-client"
exit 1
fi