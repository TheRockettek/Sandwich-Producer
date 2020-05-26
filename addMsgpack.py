import re
import os

# Converts structures with json to also include a msgpack copy converting
# GuildID string \`json:"guild_id"\`
# into
# GuildID string \`json:"guild_id" msgpack:"guild_id"\`
# and will ignore ones that have already got msgpack

rgx = re.compile(r"`((json|msgpack):\"(\S+)\")\s?((json|msgpack):\"(\S+)\")?`")

def convert(string):
    done = []
    results = re.findall(rgx, string)
    for result in results:
        # ('json:"d"', 'json', 'd', 'msgpack:"-"', 'msgpack', '-')
        # ('json:"last_message_id"', 'json', 'last_message_id', '', '', '')
        # Check there is no msgpack
        if result[4] != "msgpack" and result[1] != "msgpack":
            _final = result[0] + f' msgpack:"{result[2]}"'
            if _final not in done:
                string = string.replace(result[0], _final)
                done.append(_final)
                print(_final)
    return string

for f in os.listdir(os.getcwd()):
    path = os.path.join(os.getcwd(), f)
    if os.path.isfile(path):
        try:
            print("-->", path)
            file = open(path, "r")
            res = convert(file.read())
            file.close()

            file = open(path, "w")
            file.write(res)
            file.close()
        except Exception as e:
            print(e)
