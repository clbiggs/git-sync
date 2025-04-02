#!/bin/bash

cd .git/hooks
if [ ! -f pre-commit ]; then
    cat << 'EOF' > pre-commit
#!/bin/sh

exec make precommit

EOF
chmod +x pre-commit
fi
