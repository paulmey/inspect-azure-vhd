# inspect-azure-vhd [![Build Status](https://travis-ci.org/paulmey/inspect-azure-vhd.svg?branch=master)](https://travis-ci.org/paulmey/inspect-azure-vhd)

This repo currently contains sample code that allows you to download individual
files from an EXT2/3/4 Linux VHD in Azure. The code is still pretty rough and very much
a partial implementation where it comes to reading EXT4. I've only tested this with a
couple of CentOS images, so YMMV. PR's welcome and please do create issues for things 
you'll want to see.

## Usage

You can either [download the binary](https://github.com/paulmey/inspect-azure-vhd/releases) or compile from source (see below). Assuming that the binary is in your path, you can do:
```
inspect-azure-vhd "https://youraccount.blob.core.windows.net/container/path/to/blob.vhd?<shared access signature>"
```
The tool wants a url that is can read without knowing your storage keys, so you'll need to create a
shared access signature to append to your uri (see below for instructions). The reason for this is that the tool is purpose-built
for technical support personnel that you may not want to share your storage account keys with.

The output will look something like this:
```
Reading partition table...
Inspecting ext4 filesystem on first partition...
Inode count:     1905008
Block count:     32676386073483008
Block size:      4096
Cluster size:    4
Blocks/group:    32768
Clusters/group:  32768
Inode/group:     8176
Magic:           61267
State:           1
FeatureCompat:   HasJournal|ExtAttr|ResizeInodes|DirIndex(0x0000003c)
FeatureIncompat: Filetype|Extents|64Bit|FlexBG(0x000002c2)
FeatureROCompat: SparseSuper|LargeFile|HugeFile|GDTCsum|DirNlink|ExtraIsize(0x0000007b)
Downloading interesting files...
   /etc/ssh/sshd_config (File) 
     \-> downloading 4443 bytes
   /etc/ssh/moduli (File) 
     \-> downloading 242153 bytes
   /etc/ssh/ssh_config (File) 
     \-> downloading 2208 bytes
   /etc/ssh/sshd_config.rpmnew (File) 
     \-> downloading 4361 bytes
   /etc/fstab (File) 
     \-> downloading 313 bytes
WARN: failed to resolve symlink /etc/mtab: DirEntry not found: /proc/self/mounts
   /etc/waagent.conf (File) 
     \-> downloading 1505 bytes
   /var/log/messages (File) 
     \-> downloading 1161694 bytes
   /var/log/boot.log (File) 
     \-> downloading 5909 bytes
   /var/log/dmesg (File) 
     \-> downloading 41794 bytes
```

## Creating a SAS (shared access signature) uri for your VHD

A Shared Access Signature (SAS) token is just a bunch of uri parameters like `se=2015-04-28T13%3A00%3A00Z&sp=r&sv=2014-02-14&sr=b&sig=40bLaEqFin6mYgskDyEv5Su61aZ%2FjgGynp3lVTkwQ7w%3D`. You can concatenate that to your blob uri, just make sure there is a `?` in between the URI and the token. 
Note that the token contains ampersands and percent signs, so you need to surround your url with qoutes when you pass it on a command line.
You can use PowerShell or the Azure cross platform command-line interface (xplat CLI) to create a SAS token.

### Using Azure CLI
One way you can do this is using the [Azure cross platfrom CLI](http://github.com/Azure/azure-xplat-cli). The CLI is installed as a node module, so to install it you need to do something like `sudo apt-get install npm && sudo npm install -g azure-cli` (for Ubuntu, similar for other distro's).
Once this is installed, you can use the snippet below to create a SAS token for the URI to your VHD blob.
```sh
# make sure you have fresh credentials and are in 'Service Management' mode
azure login
azure config mode asm
# if you have multiple subscriptions, select the correct one
azure account list
azure account set <subscription id or name>
# then get a key for your storage, either primary or secondary is fine
azure storage account keys list <storage account name>
# finally, create the signature. The expiry date/time at the end is in UTC
azure storage blob sas create -a <storage account name> -k <key> <containername> <blob path within container> r 2015-09-01T13:00:00
```

### Using PowerShell
If you have [Azure Powershell installed](https://azure.microsoft.com/en-us/documentation/articles/powershell-install-configure/), you can use that to create a SAS token. Assuming you have already logged in and connected to the right subscription (see link in previous sentence), these are the steps to create a SAS token:
```PowerShell
$keys = Get-AzureStorageKey $storage_account_name
$ctx = New-AzureStorageContext $storage_account_name $keys.Primary
$sastoken = New-AzureStorageBlobSASToken -Context $ctx -Container $container -Blob $blob_path_within_container -Permission r -ExpiryTime 2015-09-01T13:00:00
```
Note that the expiry date/time at the end is in UTC.

## Building from source

Assuming you have [go 1.4 toolset installed](http://golang.org/doc/install#install) and have your $GOBIN in your path:
```
go get github.com/paulmey/inspect-azure-vhd
```
