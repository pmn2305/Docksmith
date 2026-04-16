import os
greeting = os.environ.get("GREETING", "hello")
name = os.environ.get("NAME", "world")
print(f"{greeting}, {name}!")
print("Running inside docksmith container.")
print('changed')
# changed
