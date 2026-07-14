UPDATE ssh_sessions SET loopback_port=NULL WHERE state IN (3,4);
