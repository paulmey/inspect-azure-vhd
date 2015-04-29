# inspect-azure-vhd

This repo currently contains sample code that allows you to download individual
files from an EXT2/3/4 Linux VHD in Azure. The code is still pretty rough (read: a big hack)
and very much apartial implementation where it comes to reading EXT4. I've only tested
this with one CentOS image, so YMMV. PR's welcome and please do create issues for things 
you'll want to see. I'll start with some clean up and moving the EXT4 and blob stuff to 
their own packages.

## Usage

Assuming you have go 1.4 installed and have your $GOBIN in your path:
```
go install github.com/paulmey/inspect-azure-vhd
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
Iterating /var/log, downloading interesting files...
    Dir        .
    Dir        ..
    File       tallylog
    File       lastlog
    File       wtmp
    File       btmp
    Dir        ppp
    File       messages ==> downloaded 1153990 bytes
    File       secure
    File       maillog
    File       spooler
    Dir        audit
    Dir        tuned
    Dir        anaconda
    File       boot.log ==> downloaded 5909 bytes
    File       dmesg ==> downloaded 42041 bytes
    File       cron
    File       dmesg.old
    File       yum.log
    File       grubby
    File       cron-20140727
    File       maillog-20140727
    File       messages-20140727
    File       secure-20140727
    File       spooler-20140727
    Dir        azure
```

## Creating a SAS (shared access signature) uri for your VHD

One way you can do this is using the [Azure xplat cli](http://github.com/Azure/azure-xplat-cli):
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
azure storage blob sas create -a <storage account name> -k <key> <container> <blob path> r 2015-06-01T13:00:00
```

The signature is just a bunch of uri parameters `se=2015-04-28T13%3A00%3A00Z&sp=r&sv=2014-02-14&sr=b&sig=40bLaEqFin6mYgskDyEv5Su61aZ%2FjgGynp3lVTkwQ7w%3D`
and you can just glue that to your blob uri with a `?` in between. 
Note that you need to surround your url with qoutes when you pass it in a shell, since it contains ampersands (`&`).
