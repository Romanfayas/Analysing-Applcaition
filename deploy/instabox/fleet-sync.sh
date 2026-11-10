#!/bin/sh
# Run on the API box: push config/code/scripts from here to the other two boxes (the API box is where you manage everything).
. "$(dirname "$0")/lib.sh"
[ "$FLEET_SELF" = api ] || { echo "run on the api box only"; exit 1; }
for role in db jobs; do
  echo "== sync -> $role"
  if [ "$role" = jobs ]; then   # jobs box builds worker+quant images: needs the whole app tree
    tar -C "$HALAL_HOME/app" --exclude=node_modules --exclude=.next --exclude=__pycache__ -cz . | rssh $role "mkdir -p $HALAL_HOME/app && cd $HALAL_HOME/app && tar -xz --no-same-owner 2>/dev/null; chmod 600 .env"
  else                           # db box only needs .env + compose + scripts
    tar -C "$HALAL_HOME/app" -cz .env deploy | rssh $role "mkdir -p $HALAL_HOME/app && cd $HALAL_HOME/app && tar -xz --no-same-owner 2>/dev/null; chmod 600 .env"
  fi
  echo "$role" | rssh $role "cat > $HALAL_HOME/role"
  { grep -v '^FLEET_SELF=' "$HOME/.config/halal/fleet.env"; echo "FLEET_SELF=$role"; } | rssh $role 'mkdir -p ~/.config/halal && umask 077 && cat > ~/.config/halal/fleet.env && chmod 600 ~/.config/halal/fleet.env'
  tar -C "$HALAL_HOME/app/deploy/instabox" --exclude=__pycache__ -cz . | rssh $role "mkdir -p $HALAL_HOME/fleet && tar -xz -C $HALAL_HOME/fleet --no-same-owner 2>/dev/null; chmod +x $HALAL_HOME/fleet/*.sh $HALAL_HOME/fleet/*.py"
  rssh $role "$HALAL_HOME/fleet/install-units.sh $role"
done
