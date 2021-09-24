# i3-ICS-Agenda
This small tool allows you to get your agenda for today off the ICS-Link to your calendar.

## Basic Usage
The Tool can be used from the commandline and has the following parameters:
```
./i3-ics-agenda -ics-url <url> -output <format> [-cal-cache-duration <duration>]

    -ics-url <url>                  required parameter that defines the URL to the ICS-Calendar
    -output <format>                required parameter that defines the output.
                                    Valid values are
                                    - current: returns the current event in the Format "[START - END] SUBJECT"
                                    - current-link: returns the first link in the location and description of the current event
                                    - next: returns the next event in the Format "[START - END] SUBJECT"
                                    - next-link: returns the first link in the location and description of the next event
                                    - agenda: returns all events for today in the format "IS_CURRENT 'START' END' 'SUMMARY' (compatible with zenity)
    -cal-cache-duration <duration>  optional parameter that defines how long the ICS will be cached locally. Defaults to 5 minutes
```

## Display Agenda with Zenity
Zenity allows you to display list so for this I decided on using a radiolist to indicate the current event:
```
ICS="https://link-to-your-ics"
AGENDA=$(./i3-ics-agenda -ics-url $ICS -output agenda)
zenity --list --radiolist --width=500 --height=300 --text='Agenda' --column='' --column='Start' --column='End' --column='Summary' $AGENDA
```

## i3bar example
To get this all working as an i3bar applet I am using the following configuration
```
#!/bin/bash

BUTTON=${button:-}
ICS="https://link-to-your-ics"

CURRENT_EVENT=$(./i3-ics-agenda -ics-url "$ICS" -output current)
NEXT_EVENT=$(./i3-ics-agenda -ics-url "$ICS" -output next)

LABEL_ICON=${icon:-$(xrescat i3xrocks.label.time )}
LABEL_COLOR=${label_color:-$(xrescat i3xrocks.label.color "#7B8394")}
VALUE_COLOR=${color:-$(xrescat i3xrocks.value.color "#D8DEE9")}
VALUE_FONT=${font:-$(xrescat i3xrocks.value.font "Source Code Pro Medium 13")}

OUTPUT="-"
if [ "$CURRENT_EVENT" == "" ] && [ "$NEXT_EVENT" == "" ]; then
    OUTPUT="No upcoming Events "
elif [ "$CURRENT_EVENT" != "" ] && [ "$NEXT_EVENT" == "" ]; then
    OUTPUT="Current: $CURRENT_EVENT "
elif [ "$CURRENT_EVENT" == "" ] && [ "$NEXT_EVENT" != "" ]; then
    OUTPUT="Upcoming: $NEXT_EVENT "
else
    OUTPUT="$CURRENT_EVENT &gt; $NEXT_EVENT "
fi

echo "<span color=\"${LABEL_COLOR}\">$LABEL_ICON</span><span font_desc=\"${VALUE_FONT}\" color=\"${VALUE_COLOR}\"> $OUTPUT</span>"

if [ "x${BUTTON}" == "x1" ]; then
    AGENDA=$(./i3-ics-agenda -ics-url $ICS -output agenda)
    /usr/bin/i3-msg -q exec "zenity --list --radiolist --width=500 --height=300 --text='Agenda' --column='' --column='Start' --column='End' --column='Summary' $AGENDA"
fi
```
