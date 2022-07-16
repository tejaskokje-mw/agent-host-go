#!/bin/bash

# Adding APT repo address & public key to system
mkdir -p /usr/local/bin/melt-go-agent/apt
touch /usr/local/bin/melt-go-agent/apt/pgp-key.public
wget -O /usr/local/bin/melt-go-agent/apt/pgp-key.public https://apt-go.melt.so/pgp-key.public
sudo touch /etc/apt/sources.list.d/melt-go.list
echo "deb [arch=all signed-by=/usr/local/bin/melt-go-agent/apt/pgp-key.public] https://apt-go.melt.so/apt-repo stable main" | sudo tee /etc/apt/sources.list.d/melt.list

# Updating apt list on system
sudo apt-get update -o Dir::Etc::sourcelist="sources.list.d/melt-go.list" -o Dir::Etc::sourceparts="-" -o APT::Get::List-Cleanup="0"

# Installing Agent
sudo apt-get install melt-go-agent-host

MELT_USER=$(whoami)
export MELT_USER

sudo su << EOSUDO


# Running Agent as a Daemon Service
touch /etc/systemd/system/meltgoservice.service

cat << EOF > /etc/systemd/system/meltgoservice.service
[Unit]
Description=Melt daemon!
[Service]
User=$MELT_USER
#Code to execute
#Can be the path to an executable or code itself
WorkingDirectory=/usr/local/bin/melt-go-agent/apt
ExecStart=/usr/local/bin/melt-go-agent/apt/executable
Type=simple
TimeoutStopSec=10
Restart=on-failure
RestartSec=5
[Install]
WantedBy=multi-user.target
EOF

if [ ! "${TARGET}" = "" ]; then

cat << EOIF > /usr/local/bin/melt-go-agent/apt/executable
#!/bin/sh
export MELT_API_TOKEN = $MELT_API_KEY
melt-go-agent-host start
EOIF

else 

cat << EOELSE > /usr/local/bin/melt-go-agent/apt/executable
#!/bin/sh
export MELT_API_TOKEN = $MELT_API_KEY
melt-go-agent-host start
EOELSE

fi

chmod 777 /usr/local/bin/melt-go-agent/apt/executable

EOSUDO

sudo systemctl daemon-reload
sudo systemctl enable meltgoservice

if [ "${MELT_AUTO_START}" = true ]; then	
    sudo systemctl start meltgoservice
fi


# Adding Cron to update + upgrade package every 5 minutes

mkdir -p /usr/local/bin/melt-go-agent/apt/cron
touch /usr/local/bin/melt-go-agent/apt/cron/melt-go.log

sudo crontab -l > cron_bkp
sudo echo "*/5 * * * * (wget -O /usr/local/bin/melt-go-agent/apt/pgp-key.public https://apt-go.melt.so/pgp-key.public && sudo apt-get update -o Dir::Etc::sourcelist='sources.list.d/melt.list' -o Dir::Etc::sourceparts='-' -o APT::Get::List-Cleanup='0' && sudo apt-get install --only-upgrade telemetry-agent-host && sudo systemctl restart meltgoservice) >> /usr/local/bin/melt-go-agent/apt/cron/melt.log 2>&1 >> /usr/local/bin/melt-go-agent/apt/cron/melt.log" >> cron_bkp
sudo crontab cron_bkp
sudo rm cron_bkp


sudo su << EOSUDO

echo '

  Melt Go Agent Installed Successfully ! Happy MELTing !!
  ----------------------------------------------------

  /usr/local/bin 
    └───melt-go-agent
            └───apt: Contains all the required components to run APT package on the system
                └───executable: Contains the script to run agent
                └───pgp-key.public: Contains copy of public key
                └───cron:
                    └───melt.log: Contains copy of public key

  /etc 
    ├─── apt
    |      └───sources.list.d
    |                └─── melt.list: Contains the APT repo entry
    └─── systemd
           └───system
                └─── meltgoservice.service: Service Entry for Melt Agent
'
EOSUDO